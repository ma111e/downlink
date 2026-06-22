package llmprovider

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrUsageLimitReached signals that a subscription provider (codex / claude-code)
// reported its plan quota as exhausted (not a transient rate limit). Callers use
// errors.Is to detect it and abort immediately instead of rotating credentials
// or retrying, which would keep hammering an already-flagged account.
var ErrUsageLimitReached = errors.New("llm provider: subscription usage limit reached")

// usageLimitError carries the originating provider, the reported reset time, and
// the raw response body for a usage-limit response. It unwraps to
// ErrUsageLimitReached so errors.Is matches across package boundaries.
type usageLimitError struct {
	provider string
	resetAt  time.Time
	body     string
}

func (e *usageLimitError) Error() string {
	return fmt.Sprintf("%s: usage limit reached (resets %s)", e.provider, e.resetAt.Format(time.RFC3339))
}

func (e *usageLimitError) Unwrap() error { return ErrUsageLimitReached }

// usageLimitPayload is the subset of an error response body we parse to detect a
// usage-limit response and recover its reset time.
type usageLimitPayload struct {
	Error struct {
		Type         string `json:"type"`
		ResetsAt     int64  `json:"resets_at"`
		ResetsInSecs int64  `json:"resets_in_seconds"`
	} `json:"error"`
}

// parseUsageLimit inspects an API error body and, if it represents a quota
// exhaustion ("usage_limit_reached"), returns a *usageLimitError. Detection is
// text-tolerant for providers (Claude) whose exact subscription payload is not
// pinned: a JSON type match is preferred, with a substring fallback.
func parseUsageLimit(provider, body string) (*usageLimitError, bool) {
	var p usageLimitPayload
	matched := false
	if err := json.Unmarshal([]byte(body), &p); err == nil {
		if p.Error.Type == "usage_limit_reached" {
			matched = true
		}
	}
	if !matched && strings.Contains(strings.ToLower(body), "usage_limit_reached") {
		matched = true
	}
	if !matched && strings.Contains(strings.ToLower(body), "usage limit") {
		matched = true
	}
	if !matched {
		return nil, false
	}

	resetAt := time.Now().Add(time.Hour) // fallback when no machine-readable reset
	switch {
	case p.Error.ResetsAt > 0:
		resetAt = time.Unix(p.Error.ResetsAt, 0)
	case p.Error.ResetsInSecs > 0:
		resetAt = time.Now().Add(time.Duration(p.Error.ResetsInSecs) * time.Second)
	}

	return &usageLimitError{provider: provider, resetAt: resetAt, body: body}, true
}
