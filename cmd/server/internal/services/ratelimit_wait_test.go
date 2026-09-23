package services

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/ma111e/downlink/pkg/llmprovider"
	log "github.com/sirupsen/logrus"
)

func TestWaitOutRateLimit(t *testing.T) {
	ctx := context.Background()
	rl := fmt.Errorf("model error: %w", &llmprovider.RateLimitedError{
		Provider: "codex",
		ResetAt:  time.Now().Add(50 * time.Millisecond),
	})

	waits := 0
	start := time.Now()
	retry, err := waitOutRateLimit(ctx, rl, &waits, log.Fields{})
	if err != nil || !retry {
		t.Fatalf("retry=%v err=%v, want retry", retry, err)
	}
	if time.Since(start) < 40*time.Millisecond {
		t.Fatal("returned before the reset")
	}
	if waits != 1 {
		t.Fatalf("waits = %d, want 1", waits)
	}

	waits = maxRateLimitWaits
	if retry, _ := waitOutRateLimit(ctx, rl, &waits, log.Fields{}); retry {
		t.Fatal("should stop once the wait budget is spent")
	}

	waits = 0
	if retry, _ := waitOutRateLimit(ctx, errors.New("boom"), &waits, log.Fields{}); retry {
		t.Fatal("non rate-limit errors must not trigger a wait")
	}

	cctx, cancel := context.WithCancel(ctx)
	cancel()
	far := &llmprovider.RateLimitedError{Provider: "codex", ResetAt: time.Now().Add(time.Hour)}
	if _, err := waitOutRateLimit(cctx, far, &waits, log.Fields{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
