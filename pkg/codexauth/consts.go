package codexauth

import "time"

const (
	issuer          = "https://auth.openai.com"
	clientID        = "app_EMoamEEZ73f0CkXaXp7hrann"
	CodexBaseURL    = "https://chatgpt.com/backend-api/codex"
	verificationURL = "https://auth.openai.com/codex/device"
	redirectURI     = "https://auth.openai.com/deviceauth/callback"

	maxWaitSeconds = 15 * 60

	StatusOK          = "ok"
	StatusAuthFailed  = "auth_failed"
	StatusRateLimited = "rate_limited"
)

const (
	minPollInterval = 3 * time.Second
	refreshSkew     = 120 * time.Second
)

// MaxWaitDuration is the maximum time to wait for a device-code login.
func MaxWaitDuration() time.Duration { return maxWaitSeconds * time.Second }

// VerificationURL is the URL the user visits to complete the device-code login.
func VerificationURL() string { return verificationURL }
