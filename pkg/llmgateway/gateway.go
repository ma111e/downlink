// Package llmgateway is the single chokepoint through which every LLM call
// in the process must pass. Its job is to bound provider concurrency
// (--max-concurrent-llm-requests) across every code path (direct analysis,
// queue-driven analysis, digest dedupe, digest summary) so the flag actually
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
	"strings"
	"sync/atomic"
	"time"

	"github.com/ma111e/downlink/pkg/llmprovider"
	"github.com/ma111e/downlink/pkg/trace"

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
	recorder       Recorder
}

// CallRecord is one completed LLM call handed to a Recorder. The gateway fills
// the fields it knows; provider/model come from WithModelInfo, the run id from
// the call context (see WithRunID).
type CallRecord struct {
	RunID        string
	Label        string
	ProviderType string
	ModelName    string
	Prompt       string
	Response     string
	Usage        llmprovider.Usage
	UsageKnown   bool
	Duration     time.Duration
	Err          error
}

// Recorder receives every LLM call that passes through the gateway. It lets the
// monitoring layer persist calls without the gateway depending on the store or
// models packages (avoiding an import cycle). Record must not block.
type Recorder interface {
	Record(CallRecord)
}

// SetRecorder installs r as the gateway's call recorder. Passing nil disables
// recording. Not safe to call concurrently with in-flight calls; set once at
// construction.
func (g *Gateway) SetRecorder(r Recorder) { g.recorder = r }

// record forwards a CallRecord to the installed recorder, if any.
func (g *Gateway) record(rec CallRecord) {
	if g.recorder != nil {
		g.recorder.Record(rec)
	}
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
	label        string
	providerType string
	modelName    string
}

// WithLabel attaches a human-readable label to the call for log correlation
// (e.g. "analyze:task=key_points", "digest:dedupe", "digest:summary").
func WithLabel(label string) CallOption {
	return func(c *callConfig) { c.label = label }
}

// WithModelInfo records the resolved provider type and model name for the call
// so the monitoring recorder can attribute tokens to a model. The gateway only
// sees an opaque Provider, so callers that have a resolved selection pass it here.
func WithModelInfo(providerType, modelName string) CallOption {
	return func(c *callConfig) {
		c.providerType = providerType
		c.modelName = modelName
	}
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
			"label":     label,
			"wait_ms":   time.Since(waitStart).Milliseconds(),
			"in_flight": g.inFlight.Load(),
			"max":       g.maxConcurrent,
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

	var (
		resp       string
		usage      llmprovider.Usage
		usageKnown bool
		err        error
	)
	if ug, ok := p.(llmprovider.UsageGenerator); ok {
		resp, usage, usageKnown, err = ug.GenerateWithUsage(ctx, prompt)
	} else {
		resp, err = p.Generate(ctx, prompt)
	}

	if trace.Enabled() {
		trace.LLM(cfg.label, prompt, resp, time.Since(callStart), err, nil)
	}
	g.record(CallRecord{
		RunID:        RunIDFromContext(ctx),
		Label:        cfg.label,
		ProviderType: cfg.providerType,
		ModelName:    cfg.modelName,
		Prompt:       prompt,
		Response:     resp,
		Usage:        usage,
		UsageKnown:   usageKnown,
		Duration:     time.Since(callStart),
		Err:          err,
	})
	return resp, err
}

// Stream opens a streaming LLM call through the gateway, drains the reader,
// and invokes onChunk for each message chunk as it arrives. The slot is held
// for the entire duration of the stream (reader open → io.EOF / error / cancel).
//
// onChunk may be nil; in that case chunks are still accumulated and returned
// as the full response string, but no per-token callback is invoked.
// Returning a non-nil error from onChunk aborts the stream and is returned.
func (g *Gateway) Stream(
	ctx context.Context,
	p llmprovider.ChatModelProvider,
	messages []*schema.Message,
	onChunk func(chunk *schema.Message) error,
	opts ...CallOption,
) (resp string, err error) {
	cfg := resolveConfig(opts)

	if acqErr := g.acquire(ctx, cfg.label); acqErr != nil {
		return "", fmt.Errorf("llm gateway acquire: %w", acqErr)
	}
	callStart := time.Now()
	defer g.release(cfg.label, callStart)

	g.totalCalls.Add(1)

	// Accumulated streamed content; declared before the trace defer so the
	// closure can capture partial output on error/cancel paths too.
	var sb []byte
	// Usage arrives on (usually) the final chunk; keep the richest seen, mirroring
	// Eino's ConcatMessages max-merge.
	var usage llmprovider.Usage
	var usageKnown bool
	defer func() {
		out := resp
		if out == "" {
			out = string(sb)
		}
		if trace.Enabled() {
			trace.LLM(cfg.label, renderMessages(messages), out, time.Since(callStart), err, nil)
		}
		g.record(CallRecord{
			RunID:        RunIDFromContext(ctx),
			Label:        cfg.label,
			ProviderType: cfg.providerType,
			ModelName:    cfg.modelName,
			Prompt:       renderMessages(messages),
			Response:     out,
			Usage:        usage,
			UsageKnown:   usageKnown,
			Duration:     time.Since(callStart),
			Err:          err,
		})
	}()

	reader, err := p.ChatModel().Stream(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("stream open (%s): %w", cfg.label, err)
	}
	defer reader.Close()

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
		if chunk != nil && chunk.ResponseMeta != nil && chunk.ResponseMeta.Usage != nil {
			u := chunk.ResponseMeta.Usage
			if u.TotalTokens >= usage.TotalTokens {
				usage = llmprovider.Usage{
					PromptTokens:     u.PromptTokens,
					CompletionTokens: u.CompletionTokens,
					TotalTokens:      u.TotalTokens,
				}
				usageKnown = true
			}
		}
		if onChunk != nil && chunk != nil {
			if cbErr := onChunk(chunk); cbErr != nil {
				return "", fmt.Errorf("stream callback (%s): %w", cfg.label, cbErr)
			}
		}
	}

	return string(sb), nil
}

// renderMessages flattens a chat message slice into a single human-readable
// prompt string for tracing (role: content, blank line between messages).
func renderMessages(messages []*schema.Message) string {
	var b strings.Builder
	for i, m := range messages {
		if m == nil {
			continue
		}
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(string(m.Role))
		b.WriteString(": ")
		b.WriteString(m.Content)
	}
	return b.String()
}
