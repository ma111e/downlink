package services

import (
	"context"
	"downlink/pkg/llmgateway"
	"downlink/pkg/llmprovider"
	"errors"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
)

type fakeLLMGateway struct {
	streamResponses []string
	streamErrs      []error
	onStream        func(call int)
	calls           int
	messageLens     []int
}

func (g *fakeLLMGateway) Generate(_ context.Context, _ llmprovider.Provider, _ string, _ ...llmgateway.CallOption) (string, error) {
	return "", errors.New("not implemented")
}

func (g *fakeLLMGateway) Stream(_ context.Context, _ llmprovider.ChatModelProvider, messages []*schema.Message, _ func(chunk *schema.Message) error, _ ...llmgateway.CallOption) (string, error) {
	call := g.calls
	g.calls++
	g.messageLens = append(g.messageLens, len(messages))
	if g.onStream != nil {
		g.onStream(call)
	}
	if call < len(g.streamErrs) && g.streamErrs[call] != nil {
		return "", g.streamErrs[call]
	}
	if call < len(g.streamResponses) {
		return g.streamResponses[call], nil
	}
	return "", nil
}

func withNoRetryBackoff(t *testing.T) {
	t.Helper()
	original := retryBackoff
	retryBackoff = func(int) time.Duration { return time.Nanosecond }
	t.Cleanup(func() {
		retryBackoff = original
	})
}

func testResolvedLLM(maxRetries int) *ResolvedLLM {
	return &ResolvedLLM{
		ProviderType: "test",
		ModelName:    "test-model",
		Timeout:      time.Second,
		MaxRetries:   maxRetries,
	}
}

func testAnalysisTask() analysisTask {
	return analysisTask{name: "key_points"}
}

func testArticleContext() *articleContext {
	return &articleContext{articleId: "article-1"}
}

func TestRunAnalysisTaskWithRetrySucceedsFirstAttempt(t *testing.T) {
	withNoRetryBackoff(t)
	gw := &fakeLLMGateway{streamResponses: []string{`{"key_points":["one"]}`}}
	server := &LLMsServer{gw: gw}

	result, err := server.runAnalysisTaskWithRetry(context.Background(), testArticleContext(), testResolvedLLM(3), testAnalysisTask(), 1, 1, nil, "prompt", nil)
	if err != nil {
		t.Fatalf("runAnalysisTaskWithRetry returned error: %v", err)
	}
	if gw.calls != 1 {
		t.Fatalf("calls = %d, want 1", gw.calls)
	}
	if got := result.taskResultJSON; got != `{"key_points":["one"]}` {
		t.Fatalf("taskResultJSON = %s", got)
	}
}

func TestRunAnalysisTaskWithRetrySucceedsAfterTransientFailure(t *testing.T) {
	withNoRetryBackoff(t)
	gw := &fakeLLMGateway{
		streamErrs:      []error{errors.New("temporary model error")},
		streamResponses: []string{"", `{"key_points":["two"]}`},
	}
	server := &LLMsServer{gw: gw}

	result, err := server.runAnalysisTaskWithRetry(context.Background(), testArticleContext(), testResolvedLLM(3), testAnalysisTask(), 1, 1, nil, "prompt", nil)
	if err != nil {
		t.Fatalf("runAnalysisTaskWithRetry returned error: %v", err)
	}
	if gw.calls != 2 {
		t.Fatalf("calls = %d, want 2", gw.calls)
	}
	if got := result.taskResultJSON; got != `{"key_points":["two"]}` {
		t.Fatalf("taskResultJSON = %s", got)
	}
}

func TestRunAnalysisTaskWithRetryReturnsFinalFailure(t *testing.T) {
	withNoRetryBackoff(t)
	gw := &fakeLLMGateway{
		streamErrs: []error{errors.New("first"), errors.New("second"), errors.New("third")},
	}
	server := &LLMsServer{gw: gw}

	_, err := server.runAnalysisTaskWithRetry(context.Background(), testArticleContext(), testResolvedLLM(3), testAnalysisTask(), 1, 1, nil, "prompt", nil)
	if err == nil {
		t.Fatal("runAnalysisTaskWithRetry returned nil error")
	}
	if gw.calls != 3 {
		t.Fatalf("calls = %d, want 3", gw.calls)
	}
}

func TestRunAnalysisTaskWithRetryRetriesTimeout(t *testing.T) {
	withNoRetryBackoff(t)
	gw := &fakeLLMGateway{
		streamErrs:      []error{context.DeadlineExceeded},
		streamResponses: []string{"", `{"key_points":["after timeout"]}`},
	}
	server := &LLMsServer{gw: gw}

	_, err := server.runAnalysisTaskWithRetry(context.Background(), testArticleContext(), testResolvedLLM(2), testAnalysisTask(), 1, 1, nil, "prompt", nil)
	if err != nil {
		t.Fatalf("runAnalysisTaskWithRetry returned error: %v", err)
	}
	if gw.calls != 2 {
		t.Fatalf("calls = %d, want 2", gw.calls)
	}
}

func TestRunAnalysisTaskWithRetryRetriesParseFailure(t *testing.T) {
	withNoRetryBackoff(t)
	gw := &fakeLLMGateway{
		streamResponses: []string{"not json", `{"key_points":["valid"]}`},
	}
	server := &LLMsServer{gw: gw}

	_, err := server.runAnalysisTaskWithRetry(context.Background(), testArticleContext(), testResolvedLLM(2), testAnalysisTask(), 1, 1, nil, "prompt", nil)
	if err != nil {
		t.Fatalf("runAnalysisTaskWithRetry returned error: %v", err)
	}
	if gw.calls != 2 {
		t.Fatalf("calls = %d, want 2", gw.calls)
	}
}

func TestRunAnalysisTaskWithRetryStopsOnCancellation(t *testing.T) {
	withNoRetryBackoff(t)
	ctx, cancel := context.WithCancel(context.Background())
	gw := &fakeLLMGateway{
		streamErrs: []error{errors.New("temporary model error")},
		onStream: func(call int) {
			if call == 0 {
				cancel()
			}
		},
	}
	server := &LLMsServer{gw: gw}

	_, err := server.runAnalysisTaskWithRetry(ctx, testArticleContext(), testResolvedLLM(3), testAnalysisTask(), 1, 1, nil, "prompt", nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if gw.calls != 1 {
		t.Fatalf("calls = %d, want 1", gw.calls)
	}
}

func TestRunAnalysisTaskWithRetryDoesNotMutateConversationHistoryAcrossAttempts(t *testing.T) {
	withNoRetryBackoff(t)
	gw := &fakeLLMGateway{
		streamResponses: []string{"not json", `{"key_points":["valid"]}`},
	}
	server := &LLMsServer{gw: gw}
	history := []*schema.Message{{Role: schema.System, Content: "persona"}}

	_, err := server.runAnalysisTaskWithRetry(context.Background(), testArticleContext(), testResolvedLLM(2), testAnalysisTask(), 1, 1, history, "prompt", nil)
	if err != nil {
		t.Fatalf("runAnalysisTaskWithRetry returned error: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("history len = %d, want 1", len(history))
	}
	if len(gw.messageLens) != 2 {
		t.Fatalf("messageLens len = %d, want 2", len(gw.messageLens))
	}
	for i, got := range gw.messageLens {
		if got != 2 {
			t.Fatalf("messageLens[%d] = %d, want 2", i, got)
		}
	}
}
