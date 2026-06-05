package claudeauth

import (
	"fmt"
	"net/http"
)

// betaHeader lists the beta features required for OAuth/subscription auth.
// Matches what Claude Code sends. interleaved-thinking / tool-streaming betas
// are omitted because downlink does not use thinking or tools.
const betaHeader = "claude-code-20250219,oauth-2025-04-20"

// ClaudeCodeSystemPrefix must be the first system block on every OAuth request.
// Anthropic rejects/500s OAuth traffic that lacks the Claude Code identity.
const ClaudeCodeSystemPrefix = "You are Claude Code, Anthropic's official CLI for Claude."

// ClaudeHeaders returns the identity headers required for every request to the
// Anthropic Messages API when authenticating with a Claude Code OAuth token.
// The Authorization: Bearer header is set by the caller, mirroring codexauth.
func ClaudeHeaders(accessToken string) http.Header {
	h := http.Header{}
	h.Set("anthropic-version", anthropicVersion)
	h.Set("anthropic-beta", betaHeader)
	h.Set("User-Agent", fmt.Sprintf("claude-cli/%s (external, cli)", claudeCodeVersion()))
	h.Set("x-app", "cli")
	return h
}
