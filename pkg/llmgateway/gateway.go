// Package llmgateway is the single chokepoint through which every LLM call
// in the process must pass. Its job is to bound provider concurrency
// (--max-concurrent-llm-requests) across every code path — direct analysis,
// queue-driven analysis, digest dedupe, digest summary — so the flag actually
// means what it says.
//
// A Gateway owns one semaphore (`chan struct{}`) sized at construction time.
// Generate and Stream acquire a slot, invoke the provider, and release in a
// deferred call so slot leaks can't happen on panic or context cancel.
// One call == one slot. A 5-task analysis pipeline holds the slot 5 separate
// times, leaving gaps between tasks available to other callers.
package llmgateway

import (
	"context"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"downlink/pkg/llmprovider"

	"github.com/cloudwego/eino/schema"
	log "github.com/sirupsen/logrus"
)

// Gateway throttles LLM calls globally.
type Gateway struct {
	sem            chan struct{}
	maxConcurrent  int
	inFlight       atomic.Int64
	waiting        atomic.Int64
	totalCalls     atomic.Int64
	totalWaitMicro atomic.Int64
}

// Stats is a snapshot of the gateway's counters.
type Stats struct {
	MaxConcurrent int
	InFlight      int64
	Waiting       int64
	TotalCalls    int64
}

// New creates a Gateway that allows at most maxConcurrent simultaneous
// LLM calls. maxConcurrent < 1 is coerced to 1.
func New(maxConcurrent int) *Gateway {
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	return &Gateway{
		sem:           make(chan struct{}, maxConcurrent),
		maxConcurrent: maxConcurrent,
	}
}

// MaxConcurrent returns the configured cap.
func (g *Gateway) MaxConcurrent() int { return g.maxConcurrent }

// Stats returns a snapshot of runtime counters.
func (g *Gateway) Stats() Stats {
	return Stats{
		MaxConcurrent: g.maxConcurrent,
		InFlight:      g.inFlight.Load(),
		Waiting:       g.waiting.Load(),
		TotalCalls:    g.totalCalls.Load(),
	}
}

// CallOption configures a single call (Generate or Stream).
type CallOption func(*callConfig)

type callConfig struct {
	label string
}

// WithLabel attaches a human-readable label to the call for log correlation
// (e.g. "analyze:task=key_points", "digest:dedupe", "digest:summary").
func WithLabel(label string) CallOption {
	return func(c *callConfig) { c.label = label }
}

func resolveConfig(opts []CallOption) callConfig {
	cfg := callConfig{label: "unlabeled"}
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

// acquire blocks until a slot is available or ctx is cancelled.
// On success, the caller MUST eventually call g.release().
func (g *Gateway) acquire(ctx context.Context, label string) error {
	g.waiting.Add(1)
	waitStart := time.Now()
	defer func() {
		g.waiting.Add(-1)
		g.totalWaitMicro.Add(time.Since(waitStart).Microseconds())
	}()

	select {
	case g.sem <- struct{}{}:
		g.inFlight.Add(1)
		log.WithFields(log.Fields{
			"label":      label,
			"wait_ms":    time.Since(waitStart).Milliseconds(),
			"in_flight":  g.inFlight.Load(),
			"max":        g.maxConcurrent,
		}).Debug("llm_gateway.acquire")
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (g *Gateway) release(label string, callStart time.Time) {
	<-g.sem
	g.inFlight.Add(-1)
	log.WithFields(log.Fields{
		"label":       label,
		"duration_ms": time.Since(callStart).Milliseconds(),
		"in_flight":   g.inFlight.Load(),
	}).Debug("llm_gateway.release")
}

// Generate makes a one-shot (non-streaming) LLM call through the gateway.
func (g *Gateway) Generate(
	ctx context.Context,
	p llmprovider.Provider,
	prompt string,
	opts ...CallOption,
) (string, error) {
	cfg := resolveConfig(opts)

	if err := g.acquire(ctx, cfg.label); err != nil {
		return "", fmt.Errorf("llm gateway acquire: %w", err)
	}
	callStart := time.Now()
	defer g.release(cfg.label, callStart)

	g.totalCalls.Add(1)
	return p.Generate(ctx, prompt)
}

// Stream opens a streaming LLM call through the gateway, drains the reader,
// and invokes onChunk for each message chunk as it arrives. The slot is held
// for the entire duration of the stream (reader open → io.EOF / error / cancel).
//
// onChunk may be nil — in that case chunks are still accumulated and returned
// as the full response string, but no per-token callback is invoked.
// Returning a non-nil error from onChunk aborts the stream and is returned.
func (g *Gateway) Stream(
	ctx context.Context,
	p llmprovider.ChatModelProvider,
	messages []*schema.Message,
	onChunk func(chunk *schema.Message) error,
	opts ...CallOption,
) (string, error) {
	cfg := resolveConfig(opts)

	if err := g.acquire(ctx, cfg.label); err != nil {
		return "", fmt.Errorf("llm gateway acquire: %w", err)
	}
	callStart := time.Now()
	defer g.release(cfg.label, callStart)

	g.totalCalls.Add(1)

	reader, err := p.ChatModel().Stream(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("stream open (%s): %w", cfg.label, err)
	}
	defer reader.Close()

	var sb []byte
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		chunk, recvErr := reader.Recv()
		if recvErr == io.EOF {
			break
		}
		if recvErr != nil {
			return "", fmt.Errorf("stream recv (%s): %w", cfg.label, recvErr)
		}
		if chunk != nil && chunk.Content != "" {
			sb = append(sb, chunk.Content...)
		}
		if onChunk != nil && chunk != nil {
			if cbErr := onChunk(chunk); cbErr != nil {
				return "", fmt.Errorf("stream callback (%s): %w", cfg.label, cbErr)
			}
		}
	}

	return string(sb), nil
}
