package codexauth

import (
	"os"
	"time"
)

const (
	defaultIssuer          = "https://auth.openai.com"
	defaultClientID        = "app_EMoamEEZ73f0CkXaXp7hrann"
	defaultCodexBaseURL    = "https://chatgpt.com/backend-api/codex"
	defaultVerificationURL = "https://auth.openai.com/codex/device"
	redirectURI            = "https://auth.openai.com/deviceauth/callback"

	maxWaitSeconds = 15 * 60

	StatusOK          = "ok"
	StatusAuthFailed  = "auth_failed"
	StatusRateLimited = "rate_limited"
)

const (
	minPollInterval = 3 * time.Second
	refreshSkew     = 120 * time.Second
)

// CodexBaseURL is the base URL for Codex Responses API requests.
// Override with DOWNLINK_CODEX_BASE_URL for testing or alternative deployments.
var CodexBaseURL = resolvedCodexBaseURL()

func resolvedIssuer() string {
	if v := os.Getenv("DOWNLINK_CODEX_ISSUER"); v != "" {
		return v
	}
	return defaultIssuer
}

func resolvedClientID() string {
	if v := os.Getenv("DOWNLINK_CODEX_CLIENT_ID"); v != "" {
		return v
	}
	return defaultClientID
}

func resolvedCodexBaseURL() string {
	if v := os.Getenv("DOWNLINK_CODEX_BASE_URL"); v != "" {
		return v
	}
	return defaultCodexBaseURL
}

// MaxWaitDuration is the maximum time to wait for a device-code login.
func MaxWaitDuration() time.Duration { return maxWaitSeconds * time.Second }

// VerificationURL is the URL the user visits to complete the device-code login.
func VerificationURL() string {
	if v := os.Getenv("DOWNLINK_CODEX_ISSUER"); v != "" {
		return v + "/codex/device"
	}
	return defaultVerificationURL
}
