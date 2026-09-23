package llmprovider

import (
	"context"
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

// Backoff used when a 429 carries no usable Retry-After. Vars so tests can
// shrink them.
var (
	rateLimitBackoffBase = 5 * time.Second
	rateLimitBackoffMax  = 2 * time.Minute
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
// the server-reported reset when known, otherwise now plus backoff.
func rateLimitResetAt(reported time.Time, attempt int) time.Time {
	if !reported.IsZero() && reported.After(time.Now()) {
		return reported
	}
	return time.Now().Add(rateLimitBackoff(attempt))
}

// waitForRateLimitReset sleeps until t (or until ctx is done), logging so long
// pauses are visible.
func waitForRateLimitReset(ctx context.Context, provider string, t time.Time, attempt int) error {
	d := time.Until(t)
	if d <= 0 {
		return ctx.Err()
	}
	log.WithFields(log.Fields{
		"provider": provider,
		"wait":     d.Round(time.Second).String(),
		"retry":    attempt,
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
