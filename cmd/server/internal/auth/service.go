package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/ma111e/downlink/cmd/server/internal/config"
	"github.com/ma111e/downlink/pkg/claudeauth"
	"github.com/ma111e/downlink/pkg/codexauth"
	"github.com/ma111e/downlink/pkg/models"
	"github.com/ma111e/downlink/pkg/protos"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// loginSession tracks the state of an in-progress device-code login.
type loginSession struct {
	deviceCode   *codexauth.DeviceCodeResponse
	providerName string
	modelName    string
	status       string // "pending" | "approved" | "expired" | "error"
	errorMsg     string
	credentialID string
	label        string
	expiresAt    time.Time
}

// claudeLoginSession tracks an in-progress Claude Code PKCE login. Unlike the
// codex device-code flow, the user pastes the code back, so the session only
// holds the PKCE verifier and CSRF state until CompleteClaudeLogin is called.
type claudeLoginSession struct {
	verifier     string
	state        string
	providerName string
	modelName    string
	expiresAt    time.Time
}

// Service implements protos.AuthServiceServer.
type Service struct {
	protos.UnimplementedAuthServiceServer

	manager       *codexauth.Manager
	claudeManager *claudeauth.Manager

	mu             sync.Mutex
	sessions       map[string]*loginSession
	claudeSessions map[string]*claudeLoginSession
}

func NewService(manager *codexauth.Manager, claudeManager *claudeauth.Manager) *Service {
	s := &Service{
		manager:        manager,
		claudeManager:  claudeManager,
		sessions:       make(map[string]*loginSession),
		claudeSessions: make(map[string]*claudeLoginSession),
	}
	go s.reapSessions()
	return s
}

// reapSessions removes expired sessions every minute.
func (s *Service) reapSessions() {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for range t.C {
		s.mu.Lock()
		for id, sess := range s.sessions {
			if time.Now().After(sess.expiresAt) {
				if sess.status == "pending" {
					sess.status = "expired"
				}
				// Keep for a few minutes so the CLI can collect the final status.
				if time.Now().After(sess.expiresAt.Add(5 * time.Minute)) {
					delete(s.sessions, id)
				}
			}
		}
		for id, sess := range s.claudeSessions {
			if time.Now().After(sess.expiresAt) {
				delete(s.claudeSessions, id)
			}
		}
		s.mu.Unlock()
	}
}

func (s *Service) StartCodexLogin(ctx context.Context, req *protos.StartCodexLoginRequest) (*protos.StartCodexLoginResponse, error) {
	providerName := req.ProviderName
	if providerName == "" {
		providerName = "codex-sub"
	}

	dc, err := codexauth.RequestDeviceCode(ctx)
	if err != nil {
		return nil, fmt.Errorf("codex: device code request failed: %w", err)
	}

	sessionID, err := randomID()
	if err != nil {
		return nil, fmt.Errorf("codex: failed to generate session ID: %w", err)
	}
	sess := &loginSession{
		deviceCode:   dc,
		providerName: providerName,
		modelName:    req.ModelName,
		status:       "pending",
		expiresAt:    time.Now().Add(codexauth.MaxWaitDuration()),
	}
	s.mu.Lock()
	s.sessions[sessionID] = sess
	s.mu.Unlock()

	go s.runLoginWorker(sessionID, sess)

	return &protos.StartCodexLoginResponse{
		SessionId:       sessionID,
		UserCode:        dc.UserCode,
		VerificationUrl: codexauth.VerificationURL(),
		ExpiresIn:       int32(codexauth.MaxWaitDuration().Seconds()),
		PollInterval:    int32(dc.Interval),
	}, nil
}

func (s *Service) runLoginWorker(sessionID string, sess *loginSession) {
	ctx, cancel := context.WithDeadline(context.Background(), sess.expiresAt)
	defer cancel()

	authCode, codeVerifier, err := codexauth.PollForAuthorization(ctx, sess.deviceCode)
	if err != nil {
		s.mu.Lock()
		if sess.status == "pending" {
			if errors.Is(err, codexauth.ErrLoginTimeout) {
				sess.status = "expired"
			} else {
				sess.status = "error"
				sess.errorMsg = err.Error()
			}
		}
		s.mu.Unlock()
		return
	}

	pair, err := codexauth.ExchangeCode(ctx, authCode, codeVerifier)
	if err != nil {
		log.WithError(err).Error("codex: token exchange failed")
		s.mu.Lock()
		sess.status = "error"
		sess.errorMsg = err.Error()
		s.mu.Unlock()
		return
	}

	fallback := fmt.Sprintf("openai-codex-oauth-%s", sessionID[:4])
	label := codexauth.LabelFromJWT(pair.AccessToken, fallback)

	credID, err := randomID()
	if err != nil {
		s.mu.Lock()
		sess.status = "error"
		sess.errorMsg = "failed to generate credential ID"
		s.mu.Unlock()
		return
	}

	cred := models.CodexCredential{
		Id:           credID,
		Label:        label,
		Priority:     0,
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		LastRefresh:  time.Now().UTC(),
		AuthMode:     "chatgpt",
		Source:       "manual:device_code",
		LastStatus:   codexauth.StatusOK,
	}

	// Ensure the provider config entry exists and persist it before adding the
	// credential. SaveConfig takes config.Mu itself, so it must be called WITHOUT
	// the lock held (config.Mu is a non-reentrant RWMutex — locking it twice in
	// one goroutine deadlocks).
	config.Mu.Lock()
	cfg := config.Config
	found := false
	for i := range cfg.Providers {
		if cfg.Providers[i].Name == sess.providerName {
			found = true
			break
		}
	}
	if !found {
		modelName := sess.modelName
		if modelName == "" {
			modelName = "codex-mini"
		}
		cfg.Providers = append(cfg.Providers, models.ProviderConfig{
			Name:         sess.providerName,
			ProviderType: "openai-codex",
			ModelName:    modelName,
			Enabled:      true,
		})
	}
	config.Mu.Unlock()

	if !found {
		// Persist the new provider entry before AddCredential touches its pool.
		if saveErr := config.SaveConfig(cfg); saveErr != nil {
			log.WithError(saveErr).Error("codex: failed to persist new provider entry")
			s.mu.Lock()
			sess.status = "error"
			sess.errorMsg = "failed to save provider config: " + saveErr.Error()
			s.mu.Unlock()
			return
		}
	}

	pool := s.manager.EnsurePool(sess.providerName)
	if err := pool.AddCredential(cred); err != nil {
		log.WithError(err).Error("codex: failed to persist new credential")
		s.mu.Lock()
		sess.status = "error"
		sess.errorMsg = "failed to save credential: " + err.Error()
		s.mu.Unlock()
		return
	}

	s.mu.Lock()
	sess.status = "approved"
	sess.credentialID = credID
	sess.label = label
	s.mu.Unlock()

	log.WithFields(log.Fields{
		"provider": sess.providerName,
		"label":    label,
		"id":       credID,
	}).Info("codex: credential registered")
}

func (s *Service) PollCodexLogin(_ context.Context, req *protos.PollCodexLoginRequest) (*protos.PollCodexLoginResponse, error) {
	s.mu.Lock()
	sess, ok := s.sessions[req.SessionId]
	if !ok {
		s.mu.Unlock()
		return &protos.PollCodexLoginResponse{
			SessionId: req.SessionId,
			Status:    "expired",
		}, nil
	}
	resp := &protos.PollCodexLoginResponse{
		SessionId:    req.SessionId,
		Status:       sess.status,
		ErrorMessage: sess.errorMsg,
		CredentialId: sess.credentialID,
		Label:        sess.label,
	}
	s.mu.Unlock()
	return resp, nil
}

// credPool is the subset of pool operations shared by the codex and claude
// credential pools, so the list/remove/priority RPCs can serve either.
type credPool interface {
	Credentials() []models.CodexCredential
	RemoveCredential(id string) error
	SetPriority(id string, priority int) error
}

// findPool locates the credential pool for a provider name in either manager.
func (s *Service) findPool(providerName string) (credPool, bool) {
	if p, ok := s.manager.Pool(providerName); ok {
		return p, true
	}
	if p, ok := s.claudeManager.Pool(providerName); ok {
		return p, true
	}
	return nil, false
}

func (s *Service) ListCodexCredentials(_ context.Context, req *protos.ListCodexCredentialsRequest) (*protos.ListCodexCredentialsResponse, error) {
	pool, ok := s.findPool(req.ProviderName)
	if !ok {
		return &protos.ListCodexCredentialsResponse{}, nil
	}
	creds := pool.Credentials()
	resp := &protos.ListCodexCredentialsResponse{}
	for _, c := range creds {
		resp.Credentials = append(resp.Credentials, &protos.CodexCredentialInfo{
			Id:              c.Id,
			Label:           c.Label,
			Priority:        int32(c.Priority),
			LastStatus:      c.LastStatus,
			LastErrorReason: c.LastErrorReason,
			AuthMode:        c.AuthMode,
			Source:          c.Source,
		})
	}
	return resp, nil
}

func (s *Service) RemoveCodexCredential(_ context.Context, req *protos.RemoveCodexCredentialRequest) (*protos.RemoveCodexCredentialResponse, error) {
	pool, ok := s.findPool(req.ProviderName)
	if !ok {
		return &protos.RemoveCodexCredentialResponse{Removed: false}, nil
	}
	if err := pool.RemoveCredential(req.CredentialId); err != nil {
		return &protos.RemoveCodexCredentialResponse{Removed: false}, nil
	}
	return &protos.RemoveCodexCredentialResponse{Removed: true}, nil
}

func (s *Service) SetCodexCredentialPriority(_ context.Context, req *protos.SetCodexCredentialPriorityRequest) (*protos.SetCodexCredentialPriorityResponse, error) {
	pool, ok := s.findPool(req.ProviderName)
	if !ok {
		return &protos.SetCodexCredentialPriorityResponse{Updated: false}, nil
	}
	if err := pool.SetPriority(req.CredentialId, int(req.Priority)); err != nil {
		return &protos.SetCodexCredentialPriorityResponse{Updated: false}, nil
	}
	return &protos.SetCodexCredentialPriorityResponse{Updated: true}, nil
}

// claudeLoginTTL bounds how long a pending Claude PKCE session is kept.
const claudeLoginTTL = 15 * time.Minute

func (s *Service) StartClaudeLogin(_ context.Context, req *protos.StartClaudeLoginRequest) (*protos.StartClaudeLoginResponse, error) {
	providerName := req.ProviderName
	if providerName == "" {
		providerName = "claude-code-sub"
	}

	verifier, challenge, err := claudeauth.GeneratePKCE()
	if err != nil {
		return nil, fmt.Errorf("claude: failed to generate PKCE: %w", err)
	}
	state, err := claudeauth.GenerateState()
	if err != nil {
		return nil, fmt.Errorf("claude: failed to generate state: %w", err)
	}
	sessionID, err := randomID()
	if err != nil {
		return nil, fmt.Errorf("claude: failed to generate session ID: %w", err)
	}

	s.mu.Lock()
	s.claudeSessions[sessionID] = &claudeLoginSession{
		verifier:     verifier,
		state:        state,
		providerName: providerName,
		modelName:    req.ModelName,
		expiresAt:    time.Now().Add(claudeLoginTTL),
	}
	s.mu.Unlock()

	return &protos.StartClaudeLoginResponse{
		SessionId:    sessionID,
		AuthorizeUrl: claudeauth.BuildAuthorizeURL(challenge, state),
	}, nil
}

func (s *Service) CompleteClaudeLogin(ctx context.Context, req *protos.CompleteClaudeLoginRequest) (*protos.CompleteClaudeLoginResponse, error) {
	s.mu.Lock()
	sess, ok := s.claudeSessions[req.SessionId]
	s.mu.Unlock()
	if !ok {
		return &protos.CompleteClaudeLoginResponse{Status: "error", ErrorMessage: "login session expired or unknown"}, nil
	}

	code, receivedState := claudeauth.SplitCallbackCode(req.Code)
	if code == "" {
		return &protos.CompleteClaudeLoginResponse{Status: "error", ErrorMessage: "no authorization code provided"}, nil
	}
	// Validate state to prevent CSRF (RFC 6749 §10.12).
	if receivedState != sess.state {
		return &protos.CompleteClaudeLoginResponse{Status: "error", ErrorMessage: "oauth state mismatch"}, nil
	}

	pair, err := claudeauth.ExchangeCode(ctx, code, receivedState, sess.verifier)
	if err != nil {
		log.WithError(err).Error("claude: token exchange failed")
		return &protos.CompleteClaudeLoginResponse{Status: "error", ErrorMessage: err.Error()}, nil
	}

	credID, err := randomID()
	if err != nil {
		return &protos.CompleteClaudeLoginResponse{Status: "error", ErrorMessage: "failed to generate credential ID"}, nil
	}

	expiresAt := pair.ExpiresAt
	cred := models.CodexCredential{
		Id:           credID,
		Label:        fmt.Sprintf("claude-code-%s", credID[:4]),
		Priority:     0,
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		ExpiresAt:    &expiresAt,
		LastRefresh:  time.Now().UTC(),
		AuthMode:     "claude",
		Source:       "manual:pkce",
		LastStatus:   claudeauth.StatusOK,
	}

	// Ensure the provider config entry exists and persist it before the pool
	// touches its credentials. SaveConfig takes config.Mu itself, so it must be
	// called WITHOUT the lock held (config.Mu is a non-reentrant RWMutex —
	// locking it twice in one goroutine deadlocks).
	config.Mu.Lock()
	cfg := config.Config
	found := false
	for i := range cfg.Providers {
		if cfg.Providers[i].Name == sess.providerName {
			found = true
			break
		}
	}
	if !found {
		modelName := sess.modelName
		if modelName == "" {
			modelName = "claude-sonnet-4-6"
		}
		cfg.Providers = append(cfg.Providers, models.ProviderConfig{
			Name:         sess.providerName,
			ProviderType: claudeauth.ProviderType,
			ModelName:    modelName,
			Enabled:      true,
		})
	}
	config.Mu.Unlock()

	if !found {
		if saveErr := config.SaveConfig(cfg); saveErr != nil {
			log.WithError(saveErr).Error("claude: failed to persist new provider entry")
			return &protos.CompleteClaudeLoginResponse{Status: "error", ErrorMessage: "failed to save provider config: " + saveErr.Error()}, nil
		}
	}

	pool := s.claudeManager.EnsurePool(sess.providerName)
	if err := pool.AddCredential(cred); err != nil {
		log.WithError(err).Error("claude: failed to persist new credential")
		return &protos.CompleteClaudeLoginResponse{Status: "error", ErrorMessage: "failed to save credential: " + err.Error()}, nil
	}

	s.mu.Lock()
	delete(s.claudeSessions, req.SessionId)
	s.mu.Unlock()

	log.WithFields(log.Fields{
		"provider": sess.providerName,
		"id":       credID,
	}).Info("claude: credential registered")

	return &protos.CompleteClaudeLoginResponse{
		Status:       "approved",
		CredentialId: credID,
		Label:        cred.Label,
	}, nil
}

// randomID generates a cryptographically random 8-byte hex string (16 chars).
func randomID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
