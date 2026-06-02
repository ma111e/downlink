package codexauth

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func makeToken(payload map[string]any) string {
	data, _ := json.Marshal(payload)
	enc := base64.RawURLEncoding.EncodeToString(data)
	return "header." + enc + ".sig"
}

func TestExpiresWithin_Expired(t *testing.T) {
	tok := makeToken(map[string]any{"exp": time.Now().Add(-1 * time.Hour).Unix()})
	if !ExpiresWithin(tok, 0) {
		t.Fatal("expected expired token to return true")
	}
}

func TestExpiresWithin_FarFuture(t *testing.T) {
	tok := makeToken(map[string]any{"exp": time.Now().Add(10 * time.Hour).Unix()})
	if ExpiresWithin(tok, 120*time.Second) {
		t.Fatal("expected far-future token to return false")
	}
}

func TestExpiresWithin_MissingExp(t *testing.T) {
	tok := makeToken(map[string]any{"sub": "user"})
	if ExpiresWithin(tok, 0) {
		t.Fatal("missing exp should return false, not true")
	}
}

func TestExpiresWithin_MalformedToken(t *testing.T) {
	if ExpiresWithin("not.a.jwt", 0) {
		t.Fatal("malformed token should return false")
	}
	if ExpiresWithin("", 0) {
		t.Fatal("empty token should return false")
	}
}

func TestLabelFromJWT_Email(t *testing.T) {
	tok := makeToken(map[string]any{"email": "user@example.com"})
	if got := LabelFromJWT(tok, "fallback"); got != "user@example.com" {
		t.Fatalf("got %q, want %q", got, "user@example.com")
	}
}

func TestLabelFromJWT_Fallback(t *testing.T) {
	tok := makeToken(map[string]any{"sub": "x"})
	if got := LabelFromJWT(tok, "fallback"); got != "fallback" {
		t.Fatalf("got %q, want fallback", got)
	}
}

func TestChatGPTAccountID_Present(t *testing.T) {
	tok := makeToken(map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "acct_abc123",
		},
	})
	if got := ChatGPTAccountID(tok); got != "acct_abc123" {
		t.Fatalf("got %q, want %q", got, "acct_abc123")
	}
}

func TestChatGPTAccountID_Missing(t *testing.T) {
	tok := makeToken(map[string]any{"sub": "x"})
	if got := ChatGPTAccountID(tok); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestChatGPTAccountID_MalformedToken(t *testing.T) {
	// Must not panic.
	got := ChatGPTAccountID("bad")
	if got != "" {
		t.Fatalf("expected empty for bad token, got %q", got)
	}
}

func TestDecodeJWTPayload_InvalidBase64(t *testing.T) {
	tok := "header." + strings.Repeat("!", 10) + ".sig"
	_, err := decodeJWTPayload(tok)
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
}
