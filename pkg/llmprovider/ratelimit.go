package llmprovider

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

// maxRateLimitRetries bounds how many times one call waits out a transient 429
// (or an all-credentials-rate-limited pool) before giving up.
const maxRateLimitRetries = 6

// ErrRateLimited marks a call that gave up because the provider is rate limited.
// Unlike ErrUsageLimitReached it is transient: callers may wait until the
// reported reset and try again.
var ErrRateLimited = errors.New("llm provider: rate limited")

// RateLimitedError carries when the provider is expected to accept calls again.
// It unwraps to ErrRateLimited.
type RateLimitedError struct {
	Provider string
	ResetAt  time.Time
	Cause    error
}

func (e *RateLimitedError) Error() string {
	msg := fmt.Sprintf("%s: rate limited until %s", e.Provider, e.ResetAt.Format(time.RFC3339))
	if e.Cause != nil {
		msg += ": " + e.Cause.Error()
	}
	return msg
}

func (e *RateLimitedError) Unwrap() error { return ErrRateLimited }

// Backoff used when a 429 carries no usable Retry-After. Vars so tests can
// shrink them.
var (
	rateLimitBackoffBase = 5 * time.Second
	rateLimitBackoffMax  = 2 * time.Minute
	rateLimitJitterFloor = 2 * time.Second
)

// rateLimitBackoff returns base*2^attempt capped at max, plus up to 20% jitter
// so concurrent tasks don't all retry at the same instant.
func rateLimitBackoff(attempt int) time.Duration {
	d := rateLimitBackoffBase
	for i := 0; i < attempt && d < rateLimitBackoffMax; i++ {
		d *= 2
	}
	d = min(d, rateLimitBackoffMax)
	return d + time.Duration(rand.Int64N(int64(d)/5+1))
}

// parseRetryAfterHeader parses a Retry-After value (delay seconds or HTTP-date).
// Returns the zero time when the header is missing or unparseable.
func parseRetryAfterHeader(header string) time.Time {
	header = strings.TrimSpace(header)
	if header == "" {
		return time.Time{}
	}
	if secs, err := strconv.Atoi(header); err == nil && secs >= 0 {
		return time.Now().Add(time.Duration(secs) * time.Second)
	}
	if t, err := http.ParseTime(header); err == nil {
		return t
	}
	return time.Time{}
}

// rateLimitResetAt picks the time a rate-limited credential becomes usable:
// the server-reported reset when known, otherwise now plus backoff for the
// credential's current 429 streak.
func rateLimitResetAt(reported time.Time, streak int) time.Time {
	if !reported.IsZero() && reported.After(time.Now()) {
		return reported
	}
	return time.Now().Add(rateLimitBackoff(streak))
}

// withWakeJitter spreads wake-ups after a shared reset so waiting calls don't
// all hit the API at the same instant and trip the limit again.
func withWakeJitter(t time.Time) time.Time {
	spread := max(time.Until(t)/2, rateLimitJitterFloor)
	return t.Add(time.Duration(rand.Int64N(int64(spread))))
}

// waitForRateLimitReset sleeps until t plus jitter, logging so long pauses are
// visible. When that lands past ctx's deadline it returns a *RateLimitedError
// right away rather than sleeping into a timeout, so the caller can wait
// outside the per-call deadline.
func waitForRateLimitReset(ctx context.Context, provider string, t time.Time, retry int) error {
	t = withWakeJitter(t)
	if dl, ok := ctx.Deadline(); ok && t.After(dl) {
		return &RateLimitedError{Provider: provider, ResetAt: t}
	}
	d := time.Until(t)
	log.WithFields(log.Fields{
		"provider": provider,
		"wait":     d.Round(time.Second).String(),
		"retry":    retry,
		"max":      maxRateLimitRetries,
	}).Warnf("%s rate limited, waiting until %s", provider, t.Format(time.TimeOnly))
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
