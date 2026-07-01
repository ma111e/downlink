package creds

import (
	"context"
	"encoding/hex"
	"testing"
	"time"

	"github.com/ma111e/downlink/pkg/claudeauth"
	"github.com/ma111e/downlink/pkg/codexauth"
	"github.com/ma111e/downlink/pkg/models"
	"github.com/ma111e/downlink/pkg/protos"
)

// newTestService builds a Service without starting the reapSessions goroutine,
// wiring real codex/claude managers backed by an in-memory config so the
// credential pools work without touching disk or the network.
func newTestService(t *testing.T, cfg *models.ServerConfig) *Service {
	t.Helper()
	get := func() *models.ServerConfig { return cfg }
	save := func(c *models.ServerConfig) error { cfg = c; return nil }
	return &Service{
		manager:        codexauth.NewManager(get, save),
		claudeManager:  claudeauth.NewManager(get, save),
		sessions:       make(map[string]*loginSession),
		claudeSessions: make(map[string]*claudeLoginSession),
	}
}

func TestRandomIDIsHex16AndUnique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id, err := randomID()
		if err != nil {
			t.Fatalf("randomID() error = %v", err)
		}
		if len(id) != 16 {
			t.Fatalf("randomID() = %q, len %d, want 16", id, len(id))
		}
		if _, err := hex.DecodeString(id); err != nil {
			t.Fatalf("randomID() = %q is not hex: %v", id, err)
		}
		if seen[id] {
			t.Fatalf("randomID() produced a duplicate: %q", id)
		}
		seen[id] = true
	}
}

func TestPollCodexLoginUnknownSessionIsExpired(t *testing.T) {
	svc := newTestService(t, &models.ServerConfig{})
	resp, err := svc.PollCodexLogin(context.Background(), &protos.PollCodexLoginRequest{SessionId: "ghost"})
	if err != nil {
		t.Fatalf("PollCodexLogin() error = %v", err)
	}
	if resp.Status != "expired" {
		t.Fatalf("Status = %q, want expired for unknown session", resp.Status)
	}
}

func TestPollCodexLoginReflectsSessionState(t *testing.T) {
	svc := newTestService(t, &models.ServerConfig{})
	svc.sessions["s1"] = &loginSession{
		status:       "approved",
		credentialID: "cred-1",
		label:        "my-label",
		expiresAt:    time.Now().Add(time.Hour),
	}
	resp, err := svc.PollCodexLogin(context.Background(), &protos.PollCodexLoginRequest{SessionId: "s1"})
	if err != nil {
		t.Fatalf("PollCodexLogin() error = %v", err)
	}
	if resp.Status != "approved" || resp.CredentialId != "cred-1" || resp.Label != "my-label" {
		t.Fatalf("resp = %+v, want approved/cred-1/my-label", resp)
	}
}

// configWithCodexProvider returns a config holding one openai-codex provider so
// the codex manager builds a pool for it.
func configWithCodexProvider(name string, creds ...models.CodexCredential) *models.ServerConfig {
	return &models.ServerConfig{
		DbPath: "/db",
		Providers: []models.ProviderConfig{
			{Name: name, ProviderType: "openai-codex", ModelName: "codex-mini", Enabled: true, Credentials: creds},
		},
	}
}

func TestFindPoolResolvesCodexAndClaudeAndMiss(t *testing.T) {
	cfg := &models.ServerConfig{
		DbPath: "/db",
		Providers: []models.ProviderConfig{
			{Name: "codex-sub", ProviderType: "openai-codex"},
			{Name: "claude-sub", ProviderType: claudeauth.ProviderType},
		},
	}
	svc := newTestService(t, cfg)

	if _, ok := svc.findPool("codex-sub"); !ok {
		t.Error("findPool(codex-sub) = false, want true")
	}
	if _, ok := svc.findPool("claude-sub"); !ok {
		t.Error("findPool(claude-sub) = false, want true")
	}
	if _, ok := svc.findPool("nope"); ok {
		t.Error("findPool(nope) = true, want false")
	}
}

func TestListCodexCredentials(t *testing.T) {
	cred := models.CodexCredential{Id: "c1", Label: "main", Priority: 2, LastStatus: codexauth.StatusOK, AuthMode: "chatgpt", Source: "manual:device_code"}
	svc := newTestService(t, configWithCodexProvider("codex-sub", cred))

	resp, err := svc.ListCodexCredentials(context.Background(), &protos.ListCodexCredentialsRequest{ProviderName: "codex-sub"})
	if err != nil {
		t.Fatalf("ListCodexCredentials() error = %v", err)
	}
	if len(resp.Credentials) != 1 {
		t.Fatalf("got %d credentials, want 1", len(resp.Credentials))
	}
	got := resp.Credentials[0]
	if got.Id != "c1" || got.Label != "main" || got.Priority != 2 || got.AuthMode != "chatgpt" {
		t.Fatalf("credential = %+v, want c1/main/2/chatgpt", got)
	}
}

func TestListCodexCredentialsUnknownProviderIsEmpty(t *testing.T) {
	svc := newTestService(t, &models.ServerConfig{})
	resp, err := svc.ListCodexCredentials(context.Background(), &protos.ListCodexCredentialsRequest{ProviderName: "ghost"})
	if err != nil {
		t.Fatalf("ListCodexCredentials() error = %v", err)
	}
	if len(resp.Credentials) != 0 {
		t.Fatalf("got %d credentials, want 0 for unknown provider", len(resp.Credentials))
	}
}

func TestRemoveCodexCredential(t *testing.T) {
	cred := models.CodexCredential{Id: "c1", Label: "main"}
	svc := newTestService(t, configWithCodexProvider("codex-sub", cred))

	t.Run("existing", func(t *testing.T) {
		resp, err := svc.RemoveCodexCredential(context.Background(), &protos.RemoveCodexCredentialRequest{ProviderName: "codex-sub", CredentialId: "c1"})
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if !resp.Removed {
			t.Fatal("Removed = false, want true for existing credential")
		}
	})
	t.Run("missing id", func(t *testing.T) {
		resp, _ := svc.RemoveCodexCredential(context.Background(), &protos.RemoveCodexCredentialRequest{ProviderName: "codex-sub", CredentialId: "nope"})
		if resp.Removed {
			t.Fatal("Removed = true, want false for missing id")
		}
	})
	t.Run("unknown provider", func(t *testing.T) {
		resp, _ := svc.RemoveCodexCredential(context.Background(), &protos.RemoveCodexCredentialRequest{ProviderName: "ghost", CredentialId: "c1"})
		if resp.Removed {
			t.Fatal("Removed = true, want false for unknown provider")
		}
	})
}

func TestSetCodexCredentialPriority(t *testing.T) {
	cred := models.CodexCredential{Id: "c1", Priority: 0}
	svc := newTestService(t, configWithCodexProvider("codex-sub", cred))

	resp, err := svc.SetCodexCredentialPriority(context.Background(), &protos.SetCodexCredentialPriorityRequest{ProviderName: "codex-sub", CredentialId: "c1", Priority: 9})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if !resp.Updated {
		t.Fatal("Updated = false, want true")
	}
	// Confirm the change actually took effect in the pool.
	list, _ := svc.ListCodexCredentials(context.Background(), &protos.ListCodexCredentialsRequest{ProviderName: "codex-sub"})
	if list.Credentials[0].Priority != 9 {
		t.Fatalf("priority = %d after update, want 9", list.Credentials[0].Priority)
	}

	miss, _ := svc.SetCodexCredentialPriority(context.Background(), &protos.SetCodexCredentialPriorityRequest{ProviderName: "codex-sub", CredentialId: "nope", Priority: 1})
	if miss.Updated {
		t.Fatal("Updated = true, want false for missing id")
	}
}

func TestCompleteClaudeLoginGuards(t *testing.T) {
	svc := newTestService(t, &models.ServerConfig{})
	svc.claudeSessions["live"] = &claudeLoginSession{
		state:        "correct-state",
		providerName: "claude-sub",
		expiresAt:    time.Now().Add(time.Hour),
	}

	t.Run("unknown session", func(t *testing.T) {
		resp, _ := svc.CompleteClaudeLogin(context.Background(), &protos.CompleteClaudeLoginRequest{SessionId: "ghost", Code: "x#correct-state"})
		if resp.Status != "error" {
			t.Fatalf("Status = %q, want error for unknown session", resp.Status)
		}
	})
	t.Run("empty code", func(t *testing.T) {
		// "#correct-state" splits to empty code, valid state.
		resp, _ := svc.CompleteClaudeLogin(context.Background(), &protos.CompleteClaudeLoginRequest{SessionId: "live", Code: "#correct-state"})
		if resp.Status != "error" {
			t.Fatalf("Status = %q, want error for empty code", resp.Status)
		}
	})
	t.Run("state mismatch (CSRF)", func(t *testing.T) {
		resp, _ := svc.CompleteClaudeLogin(context.Background(), &protos.CompleteClaudeLoginRequest{SessionId: "live", Code: "realcode#wrong-state"})
		if resp.Status != "error" {
			t.Fatalf("Status = %q, want error for state mismatch", resp.Status)
		}
		if resp.ErrorMessage != "oauth state mismatch" {
			t.Fatalf("ErrorMessage = %q, want 'oauth state mismatch'", resp.ErrorMessage)
		}
	})
}
