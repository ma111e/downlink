package claudeauth

import (
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	defaultClientID     = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	defaultAuthorizeURL = "https://claude.ai/oauth/authorize"
	defaultTokenURL     = "https://console.anthropic.com/v1/oauth/token"
	defaultRedirectURI  = "https://console.anthropic.com/oauth/code/callback"
	defaultScopes       = "org:create_api_key user:profile user:inference"

	defaultAPIBaseURL = "https://api.anthropic.com"

	// anthropicVersion is the API version header value required on every request.
	anthropicVersion = "2023-06-01"

	StatusOK          = "ok"
	StatusAuthFailed  = "auth_failed"
	StatusRateLimited = "rate_limited"
)

const (
	refreshSkew = 120 * time.Second

	// claudeCodeVersionFallback is the user-agent version reported when a local
	// Claude Code install cannot be detected. Anthropic's OAuth infrastructure
	// validates the user-agent version and rejects requests that are too old, so
	// keep this reasonably current.
	claudeCodeVersionFallback = "2.1.74"
)

// APIBaseURL is the base URL for Anthropic Messages API requests.
// Override with DOWNLINK_CLAUDE_BASE_URL for testing or alternative deployments.
var APIBaseURL = resolvedAPIBaseURL()

func resolvedClientID() string {
	if v := os.Getenv("DOWNLINK_CLAUDE_CLIENT_ID"); v != "" {
		return v
	}
	return defaultClientID
}

func resolvedAuthorizeURL() string {
	if v := os.Getenv("DOWNLINK_CLAUDE_AUTHORIZE_URL"); v != "" {
		return v
	}
	return defaultAuthorizeURL
}

func resolvedTokenURL() string {
	if v := os.Getenv("DOWNLINK_CLAUDE_TOKEN_URL"); v != "" {
		return v
	}
	return defaultTokenURL
}

func resolvedRedirectURI() string {
	if v := os.Getenv("DOWNLINK_CLAUDE_REDIRECT_URI"); v != "" {
		return v
	}
	return defaultRedirectURI
}

func resolvedScopes() string {
	if v := os.Getenv("DOWNLINK_CLAUDE_SCOPES"); v != "" {
		return v
	}
	return defaultScopes
}

func resolvedAPIBaseURL() string {
	if v := os.Getenv("DOWNLINK_CLAUDE_BASE_URL"); v != "" {
		return v
	}
	return defaultAPIBaseURL
}

var (
	versionOnce  sync.Once
	versionCache string
)

// claudeCodeVersion lazily detects the installed Claude Code version for the
// spoofed user-agent, falling back to a static constant.
func claudeCodeVersion() string {
	versionOnce.Do(func() {
		versionCache = detectClaudeCodeVersion()
	})
	return versionCache
}

func detectClaudeCodeVersion() string {
	for _, name := range []string{"claude", "claude-code"} {
		out, err := exec.Command(name, "--version").Output()
		if err != nil {
			continue
		}
		fields := strings.Fields(string(out))
		if len(fields) > 0 && len(fields[0]) > 0 && fields[0][0] >= '0' && fields[0][0] <= '9' {
			return fields[0]
		}
	}
	return claudeCodeVersionFallback
}
