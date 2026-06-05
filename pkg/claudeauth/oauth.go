package claudeauth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// TokenPair holds the OAuth tokens and the computed absolute expiry.
type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

// GeneratePKCE returns a PKCE code_verifier and its S256 code_challenge.
func GeneratePKCE() (verifier, challenge string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

// GenerateState returns a random CSRF state value, independent of the verifier.
func GenerateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// BuildAuthorizeURL builds the browser authorization URL. The verifier is never
// placed in the URL; only its S256 challenge is. State is an independent value.
func BuildAuthorizeURL(challenge, state string) string {
	params := url.Values{
		"code":                  {"true"},
		"client_id":             {resolvedClientID()},
		"response_type":         {"code"},
		"redirect_uri":          {resolvedRedirectURI()},
		"scope":                 {resolvedScopes()},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {state},
	}
	return resolvedAuthorizeURL() + "?" + params.Encode()
}

// SplitCallbackCode splits the user-pasted "<code>#<state>" string.
func SplitCallbackCode(pasted string) (code, state string) {
	parts := strings.SplitN(strings.TrimSpace(pasted), "#", 2)
	code = parts[0]
	if len(parts) > 1 {
		state = parts[1]
	}
	return code, state
}

// ExchangeCode exchanges the authorization code (with its PKCE verifier) for
// tokens. The caller is responsible for validating the returned state against
// the state it generated before calling this.
func ExchangeCode(ctx context.Context, code, state, verifier string) (*TokenPair, error) {
	body, _ := json.Marshal(map[string]string{
		"grant_type":    "authorization_code",
		"client_id":     resolvedClientID(),
		"code":          code,
		"state":         state,
		"redirect_uri":  resolvedRedirectURI(),
		"code_verifier": verifier,
	})

	tctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(tctx, http.MethodPost, resolvedTokenURL(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", fmt.Sprintf("claude-cli/%s (external, cli)", claudeCodeVersion()))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed: HTTP %d: %s", resp.StatusCode, string(raw))
	}
	return parseTokenResponse(raw, "")
}

// RefreshTokens exchanges a refresh token for a new access token (and possibly a
// new refresh token). Returns ErrReloginRequired when the refresh token is no
// longer valid.
func RefreshTokens(ctx context.Context, refreshToken string) (*TokenPair, error) {
	body, _ := json.Marshal(map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
		"client_id":     resolvedClientID(),
	})

	tctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(tctx, http.MethodPost, resolvedTokenURL(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", fmt.Sprintf("claude-cli/%s (external, cli)", claudeCodeVersion()))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, parseRefreshError(resp.StatusCode, raw)
	}
	// Keep the old refresh token if the response omits a new one.
	return parseTokenResponse(raw, refreshToken)
}

// parseTokenResponse decodes a token endpoint response and computes ExpiresAt.
// fallbackRefresh is returned when the response omits a refresh_token.
func parseTokenResponse(raw []byte, fallbackRefresh string) (*TokenPair, error) {
	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("token response is not valid JSON: %w", err)
	}
	if result.AccessToken == "" {
		return nil, fmt.Errorf("token response missing access_token")
	}
	expiresIn := result.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	refresh := result.RefreshToken
	if refresh == "" {
		refresh = fallbackRefresh
	}
	return &TokenPair{
		AccessToken:  result.AccessToken,
		RefreshToken: refresh,
		ExpiresAt:    time.Now().Add(time.Duration(expiresIn) * time.Second),
	}, nil
}

// reloginErrors is the set of OAuth error codes that require re-authentication.
var reloginErrors = map[string]bool{
	"invalid_grant":        true,
	"invalid_token":        true,
	"invalid_request":      true,
	"refresh_token_reused": true,
}

func parseRefreshError(statusCode int, body []byte) error {
	if statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden {
		return fmt.Errorf("%w: HTTP %d", ErrReloginRequired, statusCode)
	}
	var oauthErr struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &oauthErr) == nil && reloginErrors[oauthErr.Error] {
		return fmt.Errorf("%w: %s", ErrReloginRequired, oauthErr.Error)
	}
	return fmt.Errorf("token refresh failed: HTTP %d: %s", statusCode, string(body))
}
