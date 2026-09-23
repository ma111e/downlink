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
	base, maxd := rateLimitBackoffBase, rateLimitBackoffMax
	rateLimitBackoffBase, rateLimitBackoffMax = 10*time.Millisecond, 50*time.Millisecond
	t.Cleanup(func() { rateLimitBackoffBase, rateLimitBackoffMax = base, maxd })
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
	var rl *codexRateLimitError
	if !errors.As(err, &rl) {
		t.Fatalf("err = %v, want rate limit error", err)
	}
	if got := int(calls.Load()); got != maxRateLimitRetries+1 {
		t.Fatalf("calls = %d, want %d", got, maxRateLimitRetries+1)
	}
}

func TestCodexWaitHonoursContext(t *testing.T) {
	p, _ := newTestCodex(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := p.Generate(ctx, "x")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want deadline exceeded", err)
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
