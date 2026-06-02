package codexauth

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func makeTokenWithAccountID(id string) string {
	payload, _ := json.Marshal(map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": id,
		},
	})
	enc := base64.RawURLEncoding.EncodeToString(payload)
	return "h." + enc + ".s"
}

func TestCodexHeaders_BaseHeaders(t *testing.T) {
	h := CodexHeaders("")
	if h.Get("User-Agent") != "codex_cli_rs/0.0.0 (downlink)" {
		t.Errorf("wrong User-Agent: %q", h.Get("User-Agent"))
	}
	if h.Get("originator") != "codex_cli_rs" {
		t.Errorf("wrong originator: %q", h.Get("originator"))
	}
}

func TestCodexHeaders_AccountID_Present(t *testing.T) {
	tok := makeTokenWithAccountID("acct_xyz")
	h := CodexHeaders(tok)
	// Must use exact casing ChatGPT-Account-ID (bypasses Go canonicalization intentionally).
	vals := h["ChatGPT-Account-ID"] //nolint:staticcheck
	if len(vals) == 0 || vals[0] != "acct_xyz" {
		t.Errorf("ChatGPT-Account-ID not set correctly: %v", vals)
	}
}

func TestCodexHeaders_AccountID_Missing(t *testing.T) {
	tok := makeToken(map[string]any{"sub": "x"})
	h := CodexHeaders(tok)
	if _, ok := h["ChatGPT-Account-ID"]; ok { //nolint:staticcheck
		t.Error("ChatGPT-Account-ID should be absent when account ID is empty")
	}
}

func TestCodexHeaders_AccountID_MalformedToken(t *testing.T) {
	h := CodexHeaders("not-a-token")
	if _, ok := h["ChatGPT-Account-ID"]; ok { //nolint:staticcheck
		t.Error("ChatGPT-Account-ID should be absent for malformed token")
	}
}

func TestCodexHeaders_Casing(t *testing.T) {
	tok := makeTokenWithAccountID("acct_123")
	h := CodexHeaders(tok)
	// Incorrect Go-canonical casings must NOT be present.
	for _, bad := range []string{"Chatgpt-Account-Id", "Chatgpt-Account-ID"} {
		if _, ok := h[bad]; ok {
			t.Errorf("header with wrong casing %q must not be set", bad)
		}
	}
}
