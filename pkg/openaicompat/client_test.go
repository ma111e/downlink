package openaicompat

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// capturedRequest records what the handler saw, for request-construction asserts.
type capturedRequest struct {
	path  string
	auth  string
	ctype string
	body  chatRequest
}

// newServer returns an httptest server that records the incoming request and
// replies with the given status and raw body.
func newServer(t *testing.T, status int, respBody string, cap *capturedRequest) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap.path = r.URL.Path
		cap.auth = r.Header.Get("Authorization")
		cap.ctype = r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &cap.body)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, respBody)
	}))
	t.Cleanup(srv.Close)
	return srv
}

const okResponse = `{"id":"x","choices":[{"message":{"role":"assistant","content":"the answer"},"finish_reason":"stop"}]}`

func TestNewClientTrimsTrailingSlash(t *testing.T) {
	c := NewClient("http://host:8080/", "m", "")
	if c.baseURL != "http://host:8080" {
		t.Fatalf("baseURL = %q, want trailing slash trimmed", c.baseURL)
	}
}

func TestCompleteReturnsAssistantContent(t *testing.T) {
	var cap capturedRequest
	srv := newServer(t, http.StatusOK, okResponse, &cap)
	c := NewClient(srv.URL, "llama-3", "secret")

	got, err := c.Complete(context.Background(), "be brief", "hello", 0.5, 256)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if got != "the answer" {
		t.Fatalf("Complete() = %q, want %q", got, "the answer")
	}

	// Request construction.
	if cap.path != "/v1/chat/completions" {
		t.Errorf("path = %q, want /v1/chat/completions", cap.path)
	}
	if cap.ctype != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", cap.ctype)
	}
	if cap.auth != "Bearer secret" {
		t.Errorf("Authorization = %q, want 'Bearer secret'", cap.auth)
	}
	if cap.body.Model != "llama-3" || cap.body.Temperature != 0.5 || cap.body.MaxTokens != 256 {
		t.Errorf("body = %+v, want model llama-3, temp 0.5, max 256", cap.body)
	}
	if len(cap.body.Messages) != 2 ||
		cap.body.Messages[0].Role != "system" || cap.body.Messages[0].Content != "be brief" ||
		cap.body.Messages[1].Role != "user" || cap.body.Messages[1].Content != "hello" {
		t.Errorf("messages = %+v, want [system:be brief, user:hello]", cap.body.Messages)
	}
}

func TestCompleteOmitsSystemMessageWhenEmpty(t *testing.T) {
	var cap capturedRequest
	srv := newServer(t, http.StatusOK, okResponse, &cap)
	c := NewClient(srv.URL, "m", "")

	if _, err := c.Complete(context.Background(), "", "hi", 0, 0); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if len(cap.body.Messages) != 1 || cap.body.Messages[0].Role != "user" {
		t.Fatalf("messages = %+v, want only the user message", cap.body.Messages)
	}
}

func TestCompleteNoAuthHeaderWhenKeyEmpty(t *testing.T) {
	var cap capturedRequest
	srv := newServer(t, http.StatusOK, okResponse, &cap)
	c := NewClient(srv.URL, "m", "")

	if _, err := c.Complete(context.Background(), "", "hi", 0, 0); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if cap.auth != "" {
		t.Fatalf("Authorization = %q, want empty when no api key", cap.auth)
	}
}

func TestCompleteNonOKStatusIsError(t *testing.T) {
	var cap capturedRequest
	srv := newServer(t, http.StatusInternalServerError, "upstream exploded", &cap)
	c := NewClient(srv.URL, "m", "")

	_, err := c.Complete(context.Background(), "", "hi", 0, 0)
	if err == nil {
		t.Fatal("Complete() error = nil, want error on 500")
	}
	if !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "upstream exploded") {
		t.Fatalf("error = %v, want status and body included", err)
	}
}

func TestCompleteEmptyChoicesIsError(t *testing.T) {
	var cap capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"x","choices":[]}`, &cap)
	c := NewClient(srv.URL, "m", "")

	_, err := c.Complete(context.Background(), "", "hi", 0, 0)
	if err == nil || !strings.Contains(err.Error(), "no choices") {
		t.Fatalf("error = %v, want 'no choices'", err)
	}
}

func TestCompleteMalformedJSONIsError(t *testing.T) {
	var cap capturedRequest
	srv := newServer(t, http.StatusOK, `{not json`, &cap)
	c := NewClient(srv.URL, "m", "")

	_, err := c.Complete(context.Background(), "", "hi", 0, 0)
	if err == nil || !strings.Contains(err.Error(), "failed to parse response") {
		t.Fatalf("error = %v, want parse failure", err)
	}
}
