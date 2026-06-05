package claudeauth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// TestPKCE_StateIsNotVerifier guards against reusing the PKCE code_verifier as
// the OAuth state (which would leak the secret verifier via the authorization
// URL). Mirrors hermes test_anthropic_oauth_pkce.py.
func TestPKCE_StateIsNotVerifier(t *testing.T) {
	verifier, challenge, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("GeneratePKCE: %v", err)
	}
	state, err := GenerateState()
	if err != nil {
		t.Fatalf("GenerateState: %v", err)
	}
	if verifier == state {
		t.Fatal("code_verifier must not equal oauth state")
	}

	authURL := BuildAuthorizeURL(challenge, state)
	if strings.Contains(authURL, verifier) {
		t.Fatal("code_verifier leaked into authorization URL")
	}

	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse auth url: %v", err)
	}
	q := u.Query()
	if q.Get("state") != state {
		t.Fatalf("state param = %q, want %q", q.Get("state"), state)
	}
	if q.Get("code_challenge") != challenge {
		t.Fatal("authorization URL must include the code_challenge")
	}
	if q.Get("code_challenge_method") != "S256" {
		t.Fatalf("code_challenge_method = %q, want S256", q.Get("code_challenge_method"))
	}
}

// TestPKCE_ChallengeMatchesVerifier verifies the S256 transform.
func TestPKCE_ChallengeMatchesVerifier(t *testing.T) {
	verifier, challenge, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("GeneratePKCE: %v", err)
	}
	sum := sha256.Sum256([]byte(verifier))
	want := base64.RawURLEncoding.EncodeToString(sum[:])
	if challenge != want {
		t.Fatalf("challenge = %q, want %q", challenge, want)
	}
}

func TestSplitCallbackCode(t *testing.T) {
	code, state := SplitCallbackCode("  the-code#the-state  ")
	if code != "the-code" || state != "the-state" {
		t.Fatalf("got code=%q state=%q", code, state)
	}
	code, state = SplitCallbackCode("only-code")
	if code != "only-code" || state != "" {
		t.Fatalf("got code=%q state=%q", code, state)
	}
}

func TestExchangeCode_Success(t *testing.T) {
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "ac",
			"refresh_token": "re",
			"expires_in":    3600,
		})
	}))
	defer srv.Close()
	t.Setenv("DOWNLINK_CLAUDE_TOKEN_URL", srv.URL)

	pair, err := ExchangeCode(context.Background(), "code123", "state123", "verifier123")
	if err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}
	if pair.AccessToken != "ac" || pair.RefreshToken != "re" {
		t.Fatalf("unexpected tokens: %+v", pair)
	}
	if pair.ExpiresAt.IsZero() {
		t.Fatal("expected ExpiresAt to be set")
	}
	if gotBody["grant_type"] != "authorization_code" || gotBody["code_verifier"] != "verifier123" {
		t.Fatalf("unexpected exchange body: %+v", gotBody)
	}
}

func TestRefreshTokens_InvalidGrantNeedsRelogin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
	}))
	defer srv.Close()
	t.Setenv("DOWNLINK_CLAUDE_TOKEN_URL", srv.URL)

	_, err := RefreshTokens(context.Background(), "rt")
	if !errors.Is(err, ErrReloginRequired) {
		t.Fatalf("expected ErrReloginRequired, got %v", err)
	}
}

func TestRefreshTokens_KeepsOldRefreshWhenOmitted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "new-ac", "expires_in": 100})
	}))
	defer srv.Close()
	t.Setenv("DOWNLINK_CLAUDE_TOKEN_URL", srv.URL)

	pair, err := RefreshTokens(context.Background(), "old-rt")
	if err != nil {
		t.Fatalf("RefreshTokens: %v", err)
	}
	if pair.RefreshToken != "old-rt" {
		t.Fatalf("expected old refresh token retained, got %q", pair.RefreshToken)
	}
	if pair.AccessToken != "new-ac" {
		t.Fatalf("expected new access token, got %q", pair.AccessToken)
	}
}
