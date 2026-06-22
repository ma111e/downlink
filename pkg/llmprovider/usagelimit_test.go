package llmprovider

import (
	"errors"
	"strconv"
	"testing"
	"time"
)

func TestParseUsageLimitCodexResetsAt(t *testing.T) {
	resetUnix := time.Now().Add(30 * time.Minute).Unix()
	body := `{"error":{"type":"usage_limit_reached","message":"limit","plan_type":"plus","resets_at":` +
		strconv.FormatInt(resetUnix, 10) + `,"resets_in_seconds":1800}}`

	ule, ok := parseUsageLimit("codex", body)
	if !ok {
		t.Fatal("parseUsageLimit returned ok=false for usage_limit_reached body")
	}
	if !errors.Is(ule, ErrUsageLimitReached) {
		t.Fatal("usageLimitError does not unwrap to ErrUsageLimitReached")
	}
	if ule.resetAt.Unix() != resetUnix {
		t.Fatalf("resetAt = %d, want %d", ule.resetAt.Unix(), resetUnix)
	}
}

func TestParseUsageLimitCodexResetsInSeconds(t *testing.T) {
	body := `{"error":{"type":"usage_limit_reached","resets_in_seconds":600}}`

	ule, ok := parseUsageLimit("codex", body)
	if !ok {
		t.Fatal("parseUsageLimit returned ok=false")
	}
	wantLB := time.Now().Add(9 * time.Minute)
	wantUB := time.Now().Add(11 * time.Minute)
	if ule.resetAt.Before(wantLB) || ule.resetAt.After(wantUB) {
		t.Fatalf("resetAt = %v, want ~10m from now", ule.resetAt)
	}
}

func TestParseUsageLimitRejectsOrdinaryRateLimit(t *testing.T) {
	body := `{"error":{"type":"rate_limit_exceeded","message":"slow down"}}`
	if _, ok := parseUsageLimit("codex", body); ok {
		t.Fatal("parseUsageLimit matched an ordinary rate-limit body")
	}
}

func TestParseUsageLimitTextFallback(t *testing.T) {
	// A non-JSON body containing the usage-limit signal still matches.
	body := `5-hour usage limit reached for your plan`
	ule, ok := parseUsageLimit("claude-code", body)
	if !ok {
		t.Fatal("parseUsageLimit did not match text fallback")
	}
	if ule.provider != "claude-code" {
		t.Fatalf("provider = %s, want claude-code", ule.provider)
	}
}
