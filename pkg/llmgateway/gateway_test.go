package llmgateway

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ma111e/downlink/pkg/llmprovider"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// --- fakes -----------------------------------------------------------------

// fakeProvider implements llmprovider.Provider only (no usage). It optionally
// blocks inside Generate until release is closed so concurrency can be observed,
// and tracks the peak number of simultaneous in-flight Generate calls.
type fakeProvider struct {
	resp string
	err  error

	// concurrency instrumentation (optional)
	entered chan struct{} // signalled once per Generate entry, if non-nil
	release chan struct{} // Generate blocks until closed, if non-nil
	cur     atomic.Int32
	peak    atomic.Int32
}

func (f *fakeProvider) Generate(_ context.Context, _ string) (string, error) {
	n := f.cur.Add(1)
	for {
		p := f.peak.Load()
		if n <= p || f.peak.CompareAndSwap(p, n) {
			break
		}
	}
	defer f.cur.Add(-1)

	if f.entered != nil {
		f.entered <- struct{}{}
	}
	if f.release != nil {
		<-f.release
	}
	return f.resp, f.err
}

// fakeUsageProvider also implements UsageGenerator.
type fakeUsageProvider struct {
	resp       string
	usage      llmprovider.Usage
	usageKnown bool
	err        error

	// plainCalled flips true if the gateway ever fell back to plain Generate,
	// which would mean the UsageGenerator assertion failed.
	plainCalled atomic.Bool
}

func (f *fakeUsageProvider) Generate(_ context.Context, _ string) (string, error) {
	f.plainCalled.Store(true)
	return f.resp, f.err
}

func (f *fakeUsageProvider) GenerateWithUsage(_ context.Context, _ string) (string, llmprovider.Usage, bool, error) {
	return f.resp, f.usage, f.usageKnown, f.err
}

// fakeChatModel implements eino's model.BaseChatModel. Stream returns whatever
// the builder produced; Generate is unused by the gateway but required by the
// interface.
type fakeChatModel struct {
	build func() (*schema.StreamReader[*schema.Message], error)
}

func (m *fakeChatModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	return schema.AssistantMessage("unused", nil), nil
}

func (m *fakeChatModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return m.build()
}

// fakeChatProvider implements llmprovider.ChatModelProvider.
type fakeChatProvider struct {
	cm model.BaseChatModel
}

func (f *fakeChatProvider) Generate(context.Context, string) (string, error) { return "", nil }
func (f *fakeChatProvider) ChatModel() model.BaseChatModel                   { return f.cm }

// chunk builds an assistant message chunk with optional usage metadata.
func chunk(content string, usage *schema.TokenUsage) *schema.Message {
	m := schema.AssistantMessage(content, nil)
	if usage != nil {
		m.ResponseMeta = &schema.ResponseMeta{Usage: usage}
	}
	return m
}

// recordingRecorder captures every CallRecord it receives.
type recordingRecorder struct {
	mu      sync.Mutex
	records []CallRecord
}

func (r *recordingRecorder) Record(rec CallRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, rec)
}

func (r *recordingRecorder) last() CallRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.records[len(r.records)-1]
}

// --- New / Stats -----------------------------------------------------------

func TestNewCoercesMaxConcurrentBelowOneToOne(t *testing.T) {
	for _, in := range []int{0, -1, -100} {
		g := New(in)
		if got := g.MaxConcurrent(); got != 1 {
			t.Errorf("New(%d).MaxConcurrent() = %d, want 1", in, got)
		}
		if cap(g.sem) != 1 {
			t.Errorf("New(%d) sem cap = %d, want 1", in, cap(g.sem))
		}
	}
}

func TestNewKeepsValidMaxConcurrent(t *testing.T) {
	g := New(5)
	if got := g.MaxConcurrent(); got != 5 {
		t.Fatalf("MaxConcurrent() = %d, want 5", got)
	}
	if cap(g.sem) != 5 {
		t.Fatalf("sem cap = %d, want 5", cap(g.sem))
	}
}

// --- Generate --------------------------------------------------------------

func TestGenerateReturnsResponseAndReleasesSlot(t *testing.T) {
	g := New(2)
	got, err := g.Generate(context.Background(), &fakeProvider{resp: "hello"}, "prompt")
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if got != "hello" {
		t.Fatalf("Generate() = %q, want %q", got, "hello")
	}
	if s := g.Stats(); s.InFlight != 0 {
		t.Fatalf("InFlight after return = %d, want 0", s.InFlight)
	}
	if s := g.Stats(); s.TotalCalls != 1 {
		t.Fatalf("TotalCalls = %d, want 1", s.TotalCalls)
	}
}

func TestGeneratePropagatesProviderErrorAndReleasesSlot(t *testing.T) {
	g := New(1)
	wantErr := errors.New("boom")
	_, err := g.Generate(context.Background(), &fakeProvider{err: wantErr}, "p")
	if !errors.Is(err, wantErr) {
		t.Fatalf("Generate() error = %v, want wrap of %v", err, wantErr)
	}
	// Slot must be released on the error path so the next call can acquire it.
	if s := g.Stats(); s.InFlight != 0 {
		t.Fatalf("InFlight after error = %d, want 0", s.InFlight)
	}
}

func TestGenerateUsesUsageGeneratorWhenAvailable(t *testing.T) {
	g := New(1)
	rec := &recordingRecorder{}
	g.SetRecorder(rec)

	p := &fakeUsageProvider{
		resp:       "answer",
		usage:      llmprovider.Usage{PromptTokens: 10, CompletionTokens: 7, TotalTokens: 17},
		usageKnown: true,
	}
	got, err := g.Generate(context.Background(), p, "q", WithLabel("analyze:key_points"))
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if got != "answer" {
		t.Fatalf("Generate() = %q, want %q", got, "answer")
	}
	if p.plainCalled.Load() {
		t.Fatal("plain Generate was called; UsageGenerator branch was skipped")
	}
	r := rec.last()
	if !r.UsageKnown || r.Usage.TotalTokens != 17 || r.Usage.PromptTokens != 10 || r.Usage.CompletionTokens != 7 {
		t.Fatalf("recorded usage = %+v known=%v, want {10 7 17} known=true", r.Usage, r.UsageKnown)
	}
	if r.Label != "analyze:key_points" {
		t.Fatalf("recorded label = %q, want %q", r.Label, "analyze:key_points")
	}
}

func TestGenerateRecordsModelInfoAndError(t *testing.T) {
	g := New(1)
	rec := &recordingRecorder{}
	g.SetRecorder(rec)

	wantErr := errors.New("provider down")
	_, _ = g.Generate(context.Background(), &fakeProvider{err: wantErr}, "p",
		WithModelInfo("openai-codex", "codex-mini"))

	r := rec.last()
	if r.ProviderType != "openai-codex" || r.ModelName != "codex-mini" {
		t.Fatalf("recorded model info = (%q,%q), want (openai-codex, codex-mini)", r.ProviderType, r.ModelName)
	}
	if !errors.Is(r.Err, wantErr) {
		t.Fatalf("recorded err = %v, want wrap of %v", r.Err, wantErr)
	}
}

func TestGenerateNilRecorderIsSafe(t *testing.T) {
	g := New(1)
	// No recorder installed; must not panic.
	if _, err := g.Generate(context.Background(), &fakeProvider{resp: "x"}, "p"); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
}

func TestGenerateCarriesRunIDFromContext(t *testing.T) {
	g := New(1)
	rec := &recordingRecorder{}
	g.SetRecorder(rec)

	ctx := WithRunID(context.Background(), "run-42")
	if _, err := g.Generate(ctx, &fakeProvider{resp: "x"}, "p"); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if got := rec.last().RunID; got != "run-42" {
		t.Fatalf("recorded RunID = %q, want %q", got, "run-42")
	}
}

// --- concurrency -----------------------------------------------------------

func TestGenerateNeverExceedsMaxConcurrent(t *testing.T) {
	const max = 3
	const callers = 12
	g := New(max)

	p := &fakeProvider{
		resp:    "ok",
		entered: make(chan struct{}, callers),
		release: make(chan struct{}),
	}

	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := g.Generate(context.Background(), p, "p"); err != nil {
				t.Errorf("Generate() error = %v", err)
			}
		}()
	}

	// Let exactly `max` callers enter the provider, then confirm no more can.
	for i := 0; i < max; i++ {
		<-p.entered
	}
	// Give any over-admitted caller a moment to (wrongly) enter.
	time.Sleep(20 * time.Millisecond)
	select {
	case <-p.entered:
		t.Fatalf("a %d+1-th caller entered the provider; concurrency cap breached", max)
	default:
	}
	if got := g.Stats().InFlight; got != max {
		t.Fatalf("InFlight = %d while saturated, want %d", got, max)
	}

	close(p.release)
	wg.Wait()

	if got := p.peak.Load(); got > max {
		t.Fatalf("peak concurrent provider calls = %d, want <= %d", got, max)
	}
	if s := g.Stats(); s.InFlight != 0 {
		t.Fatalf("InFlight after drain = %d, want 0", s.InFlight)
	}
	if s := g.Stats(); s.TotalCalls != callers {
		t.Fatalf("TotalCalls = %d, want %d", s.TotalCalls, callers)
	}
}

func TestAcquireRespectsContextCancellation(t *testing.T) {
	g := New(1)
	blocker := &fakeProvider{resp: "ok", entered: make(chan struct{}, 1), release: make(chan struct{})}

	// Occupy the single slot.
	go func() { _, _ = g.Generate(context.Background(), blocker, "p") }()
	<-blocker.entered

	// A second caller with a cancelled context must give up waiting.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := g.Generate(ctx, &fakeProvider{resp: "never"}, "p")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Generate() error = %v, want context.Canceled", err)
	}

	close(blocker.release)
}

// --- Stream ----------------------------------------------------------------

func TestStreamAccumulatesChunksAndUsage(t *testing.T) {
	g := New(1)
	rec := &recordingRecorder{}
	g.SetRecorder(rec)

	cm := &fakeChatModel{build: func() (*schema.StreamReader[*schema.Message], error) {
		return schema.StreamReaderFromArray([]*schema.Message{
			chunk("Hello, ", &schema.TokenUsage{PromptTokens: 5, TotalTokens: 5}),
			chunk("world", &schema.TokenUsage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8}),
		}), nil
	}}
	p := &fakeChatProvider{cm: cm}

	var collected []string
	resp, err := g.Stream(context.Background(), p,
		[]*schema.Message{schema.UserMessage("hi")},
		func(c *schema.Message) error { collected = append(collected, c.Content); return nil },
	)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if resp != "Hello, world" {
		t.Fatalf("Stream() = %q, want %q", resp, "Hello, world")
	}
	if len(collected) != 2 || collected[0] != "Hello, " || collected[1] != "world" {
		t.Fatalf("onChunk received %v, want [Hello,  world]", collected)
	}
	// Usage max-merges to the richest (highest TotalTokens) chunk.
	r := rec.last()
	if !r.UsageKnown || r.Usage.TotalTokens != 8 || r.Usage.CompletionTokens != 3 {
		t.Fatalf("recorded usage = %+v known=%v, want total=8 completion=3", r.Usage, r.UsageKnown)
	}
	if r.Response != "Hello, world" {
		t.Fatalf("recorded response = %q, want %q", r.Response, "Hello, world")
	}
}

func TestStreamNilOnChunkStillAccumulates(t *testing.T) {
	g := New(1)
	cm := &fakeChatModel{build: func() (*schema.StreamReader[*schema.Message], error) {
		return schema.StreamReaderFromArray([]*schema.Message{
			chunk("a", nil), chunk("b", nil), chunk("c", nil),
		}), nil
	}}
	resp, err := g.Stream(context.Background(), &fakeChatProvider{cm: cm},
		[]*schema.Message{schema.UserMessage("x")}, nil)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if resp != "abc" {
		t.Fatalf("Stream() = %q, want %q", resp, "abc")
	}
}

func TestStreamOnChunkErrorAborts(t *testing.T) {
	g := New(1)
	cm := &fakeChatModel{build: func() (*schema.StreamReader[*schema.Message], error) {
		return schema.StreamReaderFromArray([]*schema.Message{
			chunk("first", nil), chunk("second", nil),
		}), nil
	}}
	stop := errors.New("stop now")
	seen := 0
	_, err := g.Stream(context.Background(), &fakeChatProvider{cm: cm},
		[]*schema.Message{schema.UserMessage("x")},
		func(*schema.Message) error { seen++; return stop },
	)
	if !errors.Is(err, stop) {
		t.Fatalf("Stream() error = %v, want wrap of %v", err, stop)
	}
	if seen != 1 {
		t.Fatalf("onChunk invoked %d times, want 1 (must abort on first error)", seen)
	}
	// Slot released even on the abort path.
	if s := g.Stats(); s.InFlight != 0 {
		t.Fatalf("InFlight after abort = %d, want 0", s.InFlight)
	}
}

func TestStreamOpenErrorPropagates(t *testing.T) {
	g := New(1)
	openErr := errors.New("cannot open")
	cm := &fakeChatModel{build: func() (*schema.StreamReader[*schema.Message], error) {
		return nil, openErr
	}}
	_, err := g.Stream(context.Background(), &fakeChatProvider{cm: cm},
		[]*schema.Message{schema.UserMessage("x")}, nil)
	if !errors.Is(err, openErr) {
		t.Fatalf("Stream() error = %v, want wrap of %v", err, openErr)
	}
	if s := g.Stats(); s.InFlight != 0 {
		t.Fatalf("InFlight after open error = %d, want 0", s.InFlight)
	}
}

// --- renderMessages --------------------------------------------------------

func TestRenderMessages(t *testing.T) {
	got := renderMessages([]*schema.Message{
		schema.SystemMessage("be brief"),
		nil, // nil entries are skipped without affecting separators
		schema.UserMessage("hello"),
	})
	want := "system: be brief\n\nuser: hello"
	if got != want {
		t.Fatalf("renderMessages() = %q, want %q", got, want)
	}
}

// --- runcontext ------------------------------------------------------------

func TestRunIDRoundTrip(t *testing.T) {
	ctx := WithRunID(context.Background(), "abc")
	if got := RunIDFromContext(ctx); got != "abc" {
		t.Fatalf("RunIDFromContext() = %q, want %q", got, "abc")
	}
}

func TestRunIDFromContextEmptyDefault(t *testing.T) {
	if got := RunIDFromContext(context.Background()); got != "" {
		t.Fatalf("RunIDFromContext(plain) = %q, want empty", got)
	}
}
