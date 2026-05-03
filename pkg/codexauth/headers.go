package codexauth

import "net/http"

// CodexHeaders returns the Cloudflare-mitigation headers required for every
// request to chatgpt.com/backend-api/codex.
func CodexHeaders(accessToken string) http.Header {
	h := http.Header{}
	h.Set("User-Agent", "codex_cli_rs/0.0.0 (downlink)")
	h.Set("originator", "codex_cli_rs")

	if id := ChatGPTAccountID(accessToken); id != "" {
		// Canonical casing required — do not change.
		h["ChatGPT-Account-ID"] = []string{id}
	}
	return h
}
