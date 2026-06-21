package store

import (
	"strings"
	"testing"
	"time"

	"github.com/ma111e/downlink/pkg/models"
)

func TestRecordLLMCallRoundTrip(t *testing.T) {
	s := newTestStore(t)

	if err := s.StartLLMRun("run-1", time.Now()); err != nil {
		t.Fatalf("StartLLMRun() error = %v", err)
	}

	prompt := strings.Repeat("summarize this article. ", 500) // compressible
	response := "here is the summary"
	in := LLMCallInput{
		RunID:            "run-1",
		Label:            "digest:summary",
		ProviderType:     "anthropic",
		ModelName:        "claude-3-5-sonnet",
		Prompt:           prompt,
		Response:         response,
		PromptTokens:     1200,
		CompletionTokens: 80,
		TotalTokens:      1280,
		TokensKnown:      true,
		DurationMs:       4200,
	}
	if err := s.RecordLLMCall(in); err != nil {
		t.Fatalf("RecordLLMCall() error = %v", err)
	}

	calls, err := s.ListLLMCallsForRun("run-1")
	if err != nil {
		t.Fatalf("ListLLMCallsForRun() error = %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	got := calls[0]
	if got.Prompt != prompt {
		t.Errorf("prompt round-trip mismatch: got %d bytes, want %d", len(got.Prompt), len(prompt))
	}
	if got.Response != response {
		t.Errorf("response round-trip mismatch: got %q, want %q", got.Response, response)
	}
	if got.TotalTokens != 1280 || !got.TokensKnown {
		t.Errorf("token fields not persisted: total=%d known=%v", got.TotalTokens, got.TokensKnown)
	}
	// Compression actually shrank the highly-repetitive prompt.
	if len(got.LLMCall.Prompt) >= len(prompt) {
		t.Errorf("expected stored prompt to be compressed: stored %d bytes >= raw %d", len(got.LLMCall.Prompt), len(prompt))
	}
}

func TestListLLMRunSummaries(t *testing.T) {
	s := newTestStore(t)

	now := time.Now()
	ac := 6
	_ = s.StoreDigest(models.Digest{Id: "digest-xyz", ArticleCount: &ac})
	_ = s.StartLLMRun("run-a", now.Add(-2*time.Hour))
	_ = s.StartLLMRun("run-b", now.Add(-1*time.Hour))
	_ = s.LinkLLMRunToDigest("run-b", "digest-xyz", "Daily Brief")

	for i := 0; i < 3; i++ {
		_ = s.RecordLLMCall(LLMCallInput{RunID: "run-b", PromptTokens: 80, CompletionTokens: 20, TotalTokens: 100, TokensKnown: true})
	}
	_ = s.RecordLLMCall(LLMCallInput{RunID: "run-a", PromptTokens: 40, CompletionTokens: 10, TotalTokens: 50, TokensKnown: true})

	summaries, err := s.ListLLMRunSummaries(10)
	if err != nil {
		t.Fatalf("ListLLMRunSummaries() error = %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(summaries))
	}
	// Newest first: run-b.
	if summaries[0].Id != "run-b" {
		t.Errorf("expected run-b first (newest), got %s", summaries[0].Id)
	}
	if summaries[0].CallCount != 3 || summaries[0].TotalTokens != 300 {
		t.Errorf("run-b rollup wrong: calls=%d tokens=%d", summaries[0].CallCount, summaries[0].TotalTokens)
	}
	if summaries[0].TotalPromptTokens != 240 || summaries[0].TotalCompletionTokens != 60 {
		t.Errorf("run-b sent/received rollup wrong: sent=%d received=%d", summaries[0].TotalPromptTokens, summaries[0].TotalCompletionTokens)
	}
	if summaries[0].Title != "Daily Brief" || summaries[0].DigestId != "digest-xyz" {
		t.Errorf("run-b digest link not applied: title=%q digest=%q", summaries[0].Title, summaries[0].DigestId)
	}
	// Article count comes from the joined digest; avg = 300 tokens / 6 articles.
	if summaries[0].ArticleCount != 6 || summaries[0].AvgTokensPerArticle() != 50 {
		t.Errorf("run-b per-article wrong: articles=%d avg=%d", summaries[0].ArticleCount, summaries[0].AvgTokensPerArticle())
	}
	// run-a has no linked digest → unknown article count, avg 0.
	if summaries[1].ArticleCount != 0 || summaries[1].AvgTokensPerArticle() != 0 {
		t.Errorf("run-a should have no article count: articles=%d avg=%d", summaries[1].ArticleCount, summaries[1].AvgTokensPerArticle())
	}
	if summaries[1].CallCount != 1 || summaries[1].TotalTokens != 50 {
		t.Errorf("run-a rollup wrong: calls=%d tokens=%d", summaries[1].CallCount, summaries[1].TotalTokens)
	}
}

func TestPruneLLMRuns(t *testing.T) {
	s := newTestStore(t)

	base := time.Now().Add(-10 * time.Hour)
	for i := 0; i < 5; i++ {
		id := string(rune('a' + i))
		_ = s.StartLLMRun("run-"+id, base.Add(time.Duration(i)*time.Hour))
		_ = s.RecordLLMCall(LLMCallInput{RunID: "run-" + id, TotalTokens: 10})
	}

	if err := s.PruneLLMRuns(2); err != nil {
		t.Fatalf("PruneLLMRuns() error = %v", err)
	}

	summaries, err := s.ListLLMRunSummaries(10)
	if err != nil {
		t.Fatalf("ListLLMRunSummaries() error = %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("expected 2 runs after prune, got %d", len(summaries))
	}
	// The two most recent (run-e, run-d) survive.
	if summaries[0].Id != "run-e" || summaries[1].Id != "run-d" {
		t.Errorf("unexpected survivors: %s, %s", summaries[0].Id, summaries[1].Id)
	}
	// Pruned runs' calls are gone too.
	calls, _ := s.ListLLMCallsForRun("run-a")
	if len(calls) != 0 {
		t.Errorf("expected calls of pruned run-a to be deleted, got %d", len(calls))
	}
}
