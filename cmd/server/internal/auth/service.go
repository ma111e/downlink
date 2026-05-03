package auth

import (
	"context"
	"downlink/cmd/server/internal/config"
	"downlink/pkg/codexauth"
	"downlink/pkg/models"
	"downlink/pkg/protos"
	"fmt"
	"math/rand"
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

// Service implements protos.AuthServiceServer.
type Service struct {
	protos.UnimplementedAuthServiceServer

	manager *codexauth.Manager

	mu       sync.Mutex
	sessions map[string]*loginSession
}

func NewService(manager *codexauth.Manager) *Service {
	s := &Service{
		manager:  manager,
		sessions: make(map[string]*loginSession),
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
				// Keep for a few minutes so the CLI can collect the status.
				if time.Now().After(sess.expiresAt.Add(5 * time.Minute)) {
					delete(s.sessions, id)
				}
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

	sessionID := randomID()
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
			sess.status = "expired"
			if err != codexauth.ErrLoginTimeout {
				sess.status = "error"
				sess.errorMsg = err.Error()
			}
		}
		s.mu.Unlock()
		return
	}

	pair, err := codexauth.ExchangeCode(ctx, authCode, codeVerifier)
	if err != nil {
		s.mu.Lock()
		sess.status = "error"
		sess.errorMsg = err.Error()
		s.mu.Unlock()
		return
	}

	fallback := fmt.Sprintf("openai-codex-oauth-%s", sessionID[:4])
	label := codexauth.LabelFromJWT(pair.AccessToken, fallback)
	credID := randomID()

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

	// Ensure the provider config entry exists.
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

func (s *Service) ListCodexCredentials(_ context.Context, req *protos.ListCodexCredentialsRequest) (*protos.ListCodexCredentialsResponse, error) {
	pool, ok := s.manager.Pool(req.ProviderName)
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
	pool, ok := s.manager.Pool(req.ProviderName)
	if !ok {
		return &protos.RemoveCodexCredentialResponse{Removed: false}, nil
	}
	if err := pool.RemoveCredential(req.CredentialId); err != nil {
		return &protos.RemoveCodexCredentialResponse{Removed: false}, nil
	}
	return &protos.RemoveCodexCredentialResponse{Removed: true}, nil
}

func (s *Service) SetCodexCredentialPriority(_ context.Context, req *protos.SetCodexCredentialPriorityRequest) (*protos.SetCodexCredentialPriorityResponse, error) {
	pool, ok := s.manager.Pool(req.ProviderName)
	if !ok {
		return &protos.SetCodexCredentialPriorityResponse{Updated: false}, nil
	}
	if err := pool.SetPriority(req.CredentialId, int(req.Priority)); err != nil {
		return &protos.SetCodexCredentialPriorityResponse{Updated: false}, nil
	}
	return &protos.SetCodexCredentialPriorityResponse{Updated: true}, nil
}

func randomID() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 8)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}
