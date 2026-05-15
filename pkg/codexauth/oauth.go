package codexauth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// flexInt unmarshals a JSON field that may be a number or a quoted number string.
// The OpenAI device-code endpoint returns "interval" as a string.
type flexInt int

func (f *flexInt) UnmarshalJSON(data []byte) error {
	var n int
	if err := json.Unmarshal(data, &n); err == nil {
		*f = flexInt(n)
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("flexInt: cannot parse %s: %w", data, err)
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("flexInt: cannot parse %q as integer: %w", s, err)
	}
	*f = flexInt(n)
	return nil
}

// DeviceCodeResponse is returned by the usercode endpoint.
type DeviceCodeResponse struct {
	UserCode     string  `json:"user_code"`
	DeviceAuthID string  `json:"device_auth_id"`
	Interval     flexInt `json:"interval"`
}

// TokenPair holds the access and refresh tokens.
type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

// RequestDeviceCode starts the device-code flow and returns the code to show the user.
func RequestDeviceCode(ctx context.Context) (*DeviceCodeResponse, error) {
	body, _ := json.Marshal(map[string]string{"client_id": resolvedClientID()})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		resolvedIssuer()+"/api/accounts/deviceauth/usercode", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device code request failed: HTTP %d", resp.StatusCode)
	}

	rawBody, _ := io.ReadAll(resp.Body)

	var dc DeviceCodeResponse
	if err := json.Unmarshal(rawBody, &dc); err != nil {
		return nil, err
	}
	if dc.UserCode == "" || dc.DeviceAuthID == "" {
		return nil, fmt.Errorf("device code response missing user_code or device_auth_id")
	}
	if int(dc.Interval) < int(minPollInterval.Seconds()) {
		dc.Interval = flexInt(minPollInterval.Seconds())
	}
	return &dc, nil
}

// PollForAuthorization polls until the user completes login or the deadline passes.
// Returns authorization_code and code_verifier on success.
func PollForAuthorization(ctx context.Context, dc *DeviceCodeResponse) (authCode, codeVerifier string, err error) {
	deadline := time.Now().Add(maxWaitSeconds * time.Second)
	interval := time.Duration(dc.Interval) * time.Second

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return "", "", ctx.Err()
		case <-time.After(interval):
		}

		body, _ := json.Marshal(map[string]string{
			"device_auth_id": dc.DeviceAuthID,
			"user_code":      dc.UserCode,
		})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			resolvedIssuer()+"/api/accounts/deviceauth/token", bytes.NewReader(body))
		if err != nil {
			return "", "", err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", "", err
		}

		rawPoll, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			var result struct {
				AuthorizationCode string `json:"authorization_code"`
				CodeVerifier      string `json:"code_verifier"`
			}
			if err := json.Unmarshal(rawPoll, &result); err != nil {
				return "", "", err
			}
			return result.AuthorizationCode, result.CodeVerifier, nil
		}

		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound {
			continue // user hasn't finished login yet
		}
		return "", "", fmt.Errorf("device auth polling failed: HTTP %d", resp.StatusCode)
	}
	return "", "", ErrLoginTimeout
}

// ExchangeCode exchanges the authorization_code+code_verifier for OAuth tokens.
func ExchangeCode(ctx context.Context, authCode, codeVerifier string) (*TokenPair, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {authCode},
		"redirect_uri":  {redirectURI},
		"client_id":     {resolvedClientID()},
		"code_verifier": {codeVerifier},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		resolvedIssuer()+"/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	rawBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed: HTTP %d: %s", resp.StatusCode, string(rawBody))
	}

	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(rawBody, &result); err != nil {
		return nil, err
	}
	if result.AccessToken == "" || result.RefreshToken == "" {
		return nil, fmt.Errorf("token response incomplete: access_token or refresh_token missing")
	}
	return &TokenPair{AccessToken: result.AccessToken, RefreshToken: result.RefreshToken}, nil
}

// RefreshTokens exchanges a refresh token for a new access token (and possibly a new refresh token).
func RefreshTokens(ctx context.Context, refreshToken string) (*TokenPair, error) {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {resolvedClientID()},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		resolvedIssuer()+"/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	rawBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, parseRefreshError(resp.StatusCode, rawBody)
	}

	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(rawBody, &result); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrReloginRequired, "refresh response is not valid JSON")
	}
	if result.AccessToken == "" {
		return nil, fmt.Errorf("%w: refresh response missing access_token", ErrReloginRequired)
	}
	out := &TokenPair{AccessToken: result.AccessToken, RefreshToken: refreshToken}
	if result.RefreshToken != "" {
		out.RefreshToken = result.RefreshToken
	}
	return out, nil
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

	// Try OAuth-shaped error: {"error":"...", "error_description":"..."}
	var oauthErr struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &oauthErr) == nil && reloginErrors[oauthErr.Error] {
		return fmt.Errorf("%w: %s", ErrReloginRequired, oauthErr.Error)
	}

	// Try OpenAI-shaped error: {"error":{"code":"...","message":"..."}}
	var openaiErr struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &openaiErr) == nil && reloginErrors[openaiErr.Error.Code] {
		return fmt.Errorf("%w: %s", ErrReloginRequired, openaiErr.Error.Code)
	}

	return fmt.Errorf("token refresh failed: HTTP %d: %s", statusCode, string(body))
}
