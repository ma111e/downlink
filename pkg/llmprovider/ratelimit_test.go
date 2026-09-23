package llmprovider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ma111e/downlink/pkg/codexauth"
	"github.com/ma111e/downlink/pkg/models"
)

const okStream = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hi\"}\n\ndata: [DONE]\n\n"

func newTestCodex(t *testing.T, handler http.HandlerFunc) (*codexProvider, *codexauth.Pool) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	pool := codexauth.NewPool(
		[]models.CodexCredential{{Id: "a", AccessToken: "tok"}},
		func([]models.CodexCredential) error { return nil },
	)
	return newCodexProviderFromPool("m", srv.URL, pool, 5*time.Second), pool
}

func shrinkBackoff(t *testing.T) {
	t.Helper()
	base, maxd, floor := rateLimitBackoffBase, rateLimitBackoffMax, rateLimitJitterFloor
	rateLimitBackoffBase, rateLimitBackoffMax, rateLimitJitterFloor = 10*time.Millisecond, 50*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { rateLimitBackoffBase, rateLimitBackoffMax, rateLimitJitterFloor = base, maxd, floor })
}

func TestCodexRetriesAfterRetryAfter(t *testing.T) {
	var calls atomic.Int32
	p, _ := newTestCodex(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"detail":"Rate limit exceeded"}`))
			return
		}
		_, _ = w.Write([]byte(okStream))
	})

	start := time.Now()
	out, err := p.Generate(context.Background(), "x")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if out != "hi" {
		t.Fatalf("out = %q", out)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls = %d, want 2", calls.Load())
	}
	if el := time.Since(start); el < 900*time.Millisecond {
		t.Fatalf("retried after %s, expected to wait ~1s", el)
	}
}

func TestCodexBacksOffWithoutRetryAfter(t *testing.T) {
	shrinkBackoff(t)
	var calls atomic.Int32
	p, _ := newTestCodex(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) <= 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(okStream))
	})

	start := time.Now()
	if _, err := p.Generate(context.Background(), "x"); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if el := time.Since(start); el > 2*time.Second {
		t.Fatalf("took %s, backoff should be short", el)
	}
}

func TestCodexGivesUpAfterMaxRetries(t *testing.T) {
	shrinkBackoff(t)
	var calls atomic.Int32
	p, _ := newTestCodex(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
	})

	_, err := p.Generate(context.Background(), "x")
	var rle *RateLimitedError
	if !errors.As(err, &rle) || !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want *RateLimitedError", err)
	}
	if got := int(calls.Load()); got != maxRateLimitRetries+1 {
		t.Fatalf("calls = %d, want %d", got, maxRateLimitRetries+1)
	}
}

func TestCodexResetPastDeadlineReturnsRateLimited(t *testing.T) {
	p, _ := newTestCodex(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	_, err := p.Generate(ctx, "x")
	var rle *RateLimitedError
	if !errors.As(err, &rle) {
		t.Fatalf("err = %v, want *RateLimitedError", err)
	}
	if time.Until(rle.ResetAt) < 55*time.Second {
		t.Fatalf("ResetAt = %v, want ~60s out", rle.ResetAt)
	}
	if el := time.Since(start); el > time.Second {
		t.Fatalf("took %s, should not sleep into the deadline", el)
	}
}

func TestCodexWaitHonoursCancel(t *testing.T) {
	p, _ := newTestCodex(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
	})

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)
	_, err := p.Generate(ctx, "x")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestMarkRateLimitedKeepsLaterReset(t *testing.T) {
	pool := codexauth.NewPool(
		[]models.CodexCredential{{Id: "a", AccessToken: "tok"}},
		func([]models.CodexCredential) error { return nil },
	)
	lease, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	late := time.Now().Add(time.Hour)
	lease.MarkRateLimited(late)
	lease.MarkRateLimited(time.Now().Add(5 * time.Second))

	got, ok := pool.NextReset()
	if !ok || !got.Equal(late) {
		t.Fatalf("NextReset = %v, want the later reset %v", got, late)
	}
	if lease.RateLimitStreak() != 2 {
		t.Fatalf("streak = %d, want 2", lease.RateLimitStreak())
	}
	lease.MarkOK()
	if lease.RateLimitStreak() != 0 {
		t.Fatal("MarkOK should clear the streak")
	}
}

func TestCodexUsageLimitDoesNotRetry(t *testing.T) {
	var calls atomic.Int32
	p, _ := newTestCodex(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"type":"usage_limit_reached","resets_in_seconds":3600}}`))
	})

	_, err := p.Generate(context.Background(), "x")
	if !errors.Is(err, ErrUsageLimitReached) {
		t.Fatalf("err = %v, want ErrUsageLimitReached", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}
}

func TestPoolNextReset(t *testing.T) {
	now := time.Now()
	early, late := now.Add(time.Minute), now.Add(time.Hour)
	earliest := now.Add(time.Second)
	pool := codexauth.NewPool([]models.CodexCredential{
		{Id: "ok", LastStatus: codexauth.StatusOK},
		{Id: "late", LastStatus: codexauth.StatusRateLimited, LastErrorResetAt: &late},
		{Id: "early", LastStatus: codexauth.StatusRateLimited, LastErrorResetAt: &early},
		{Id: "dead", LastStatus: codexauth.StatusAuthFailed, LastErrorResetAt: &earliest},
	}, func([]models.CodexCredential) error { return nil })

	got, ok := pool.NextReset()
	if !ok || !got.Equal(early) {
		t.Fatalf("NextReset = %v, %v; want %v", got, ok, early)
	}

	empty := codexauth.NewPool([]models.CodexCredential{{Id: "ok"}}, nil)
	if _, ok := empty.NextReset(); ok {
		t.Fatal("NextReset reported a reset with no rate-limited credentials")
	}
}

func TestParseRetryAfterHeader(t *testing.T) {
	if !parseRetryAfterHeader("").IsZero() {
		t.Fatal("empty header should give zero time")
	}
	if !parseRetryAfterHeader("soon").IsZero() {
		t.Fatal("garbage header should give zero time")
	}
	if d := time.Until(parseRetryAfterHeader("30")); d < 29*time.Second || d > 31*time.Second {
		t.Fatalf("seconds header: %s", d)
	}
	want := time.Now().Add(2 * time.Minute).UTC().Truncate(time.Second)
	if got := parseRetryAfterHeader(want.Format(http.TimeFormat)); !got.Equal(want) {
		t.Fatalf("http-date header: got %v want %v", got, want)
	}
}
