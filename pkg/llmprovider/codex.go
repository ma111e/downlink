package llmprovider

import (
	"bytes"
	"context"
	"downlink/pkg/codexauth"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// codexProvider calls the Codex Responses API (not Chat Completions) and uses
// a codexauth.Pool for token management and credential rotation.
type codexProvider struct {
	modelName string
	pool      *codexauth.Pool
	baseURL   string
	timeout   time.Duration
}

func newCodexProviderFromPool(modelName, baseURL string, pool *codexauth.Pool, timeout time.Duration) *codexProvider {
	if baseURL == "" {
		baseURL = codexauth.CodexBaseURL
	}
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	return &codexProvider{
		modelName: modelName,
		pool:      pool,
		baseURL:   strings.TrimRight(baseURL, "/"),
		timeout:   timeout,
	}
}

// Generate implements Provider.
func (p *codexProvider) Generate(ctx context.Context, prompt string) (string, error) {
	msgs := []*schema.Message{{Role: schema.User, Content: prompt}}
	resp, err := p.generateMessages(ctx, msgs)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// ChatModel implements ChatModelProvider.
func (p *codexProvider) ChatModel() model.BaseChatModel {
	return &codexChatModel{provider: p}
}

// generateMessages is the shared implementation for both Generate and the
// BaseChatModel adapter.
func (p *codexProvider) generateMessages(ctx context.Context, msgs []*schema.Message) (*schema.Message, error) {
	lease, err := p.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}

	resp, err := p.callAPI(ctx, lease, msgs)
	if err == nil {
		lease.MarkOK()
		return resp, nil
	}

	// On 401/403: force-refresh once and retry.
	if isAuthError(err) {
		lease.MarkAuthFailed(err.Error())
		retryLease, err2 := p.pool.Acquire(ctx)
		if err2 != nil {
			return nil, fmt.Errorf("codex: auth failed and no healthy credential for retry: %w", err)
		}
		resp, err2 = p.callAPI(ctx, retryLease, msgs)
		if err2 != nil {
			retryLease.MarkAuthFailed(err2.Error())
			return nil, fmt.Errorf("%w: %s", codexauth.ErrReloginRequired, err2.Error())
		}
		retryLease.MarkOK()
		return resp, nil
	}

	// On 429/rate-limit: rotate and retry once.
	if isRateLimit(err) {
		resetAt := time.Now().Add(time.Hour) // conservative default
		lease.MarkRateLimited(resetAt)
		retryLease, err2 := p.pool.Acquire(ctx)
		if err2 != nil {
			return nil, fmt.Errorf("codex: rate limited and no other credential available: %w", err)
		}
		resp, err2 = p.callAPI(ctx, retryLease, msgs)
		if err2 != nil {
			if isRateLimit(err2) {
				retryLease.MarkRateLimited(resetAt)
			}
			return nil, err2
		}
		retryLease.MarkOK()
		return resp, nil
	}

	return nil, err
}

// responsesRequest is the body sent to the Codex Responses API.
type responsesRequest struct {
	Model string           `json:"model"`
	Input []responsesInput `json:"input"`
	Store bool             `json:"store"` // always false per guide
}

type responsesInput struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// responsesResponse is the shape of a successful Codex Responses API reply.
type responsesResponse struct {
	Output []struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
}

func (p *codexProvider) callAPI(ctx context.Context, lease *codexauth.Lease, msgs []*schema.Message) (*schema.Message, error) {
	inputs := make([]responsesInput, len(msgs))
	for i, m := range msgs {
		inputs[i] = responsesInput{Role: string(m.Role), Content: m.Content}
	}

	body, err := json.Marshal(responsesRequest{
		Model: p.modelName,
		Input: inputs,
		Store: false,
	})
	if err != nil {
		return nil, err
	}

	reqCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost,
		p.baseURL+"/responses", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+lease.AccessToken)
	for k, vs := range lease.Headers {
		for _, v := range vs {
			req.Header.Set(k, v)
		}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	rawBody, _ := io.ReadAll(resp.Body)

	switch resp.StatusCode {
	case http.StatusOK:
		// fall through
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, &codexAPIError{statusCode: resp.StatusCode, body: string(rawBody)}
	case http.StatusTooManyRequests:
		ra := parseRetryAfter(resp.Header.Get("Retry-After"))
		return nil, &codexRateLimitError{resetAt: ra, body: string(rawBody)}
	default:
		return nil, fmt.Errorf("codex API error: HTTP %d: %s", resp.StatusCode, string(rawBody))
	}

	var result responsesResponse
	if err := json.Unmarshal(rawBody, &result); err != nil {
		return nil, fmt.Errorf("codex: failed to parse response: %w", err)
	}

	var text strings.Builder
	for _, out := range result.Output {
		for _, c := range out.Content {
			text.WriteString(c.Text)
		}
	}

	return &schema.Message{Role: schema.Assistant, Content: text.String()}, nil
}

// ---------------------------------------------------------------------------
// Error types for routing logic
// ---------------------------------------------------------------------------

type codexAPIError struct {
	statusCode int
	body       string
}

func (e *codexAPIError) Error() string {
	return fmt.Sprintf("codex: HTTP %d: %s", e.statusCode, e.body)
}

type codexRateLimitError struct {
	resetAt time.Time
	body    string
}

func (e *codexRateLimitError) Error() string {
	return fmt.Sprintf("codex: rate limited (reset %s): %s", e.resetAt.Format(time.RFC3339), e.body)
}

func isAuthError(err error) bool {
	if e, ok := err.(*codexAPIError); ok {
		return e.statusCode == http.StatusUnauthorized || e.statusCode == http.StatusForbidden
	}
	return false
}

func isRateLimit(err error) bool {
	_, ok := err.(*codexRateLimitError)
	return ok
}

func parseRetryAfter(header string) time.Time {
	if header == "" {
		return time.Now().Add(time.Hour)
	}
	if secs, err := strconv.Atoi(header); err == nil {
		return time.Now().Add(time.Duration(secs) * time.Second)
	}
	return time.Now().Add(time.Hour)
}

// ---------------------------------------------------------------------------
// Eino BaseChatModel adapter
// ---------------------------------------------------------------------------

type codexChatModel struct {
	provider *codexProvider
}

func (m *codexChatModel) Generate(ctx context.Context, msgs []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return m.provider.generateMessages(ctx, msgs)
}

func (m *codexChatModel) Stream(ctx context.Context, msgs []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	// Responses API streaming not implemented; fall back to non-streaming.
	msg, err := m.provider.generateMessages(ctx, msgs)
	if err != nil {
		return nil, err
	}
	sr, sw := schema.Pipe[*schema.Message](1)
	sw.Send(msg, nil)
	sw.Close()
	return sr, nil
}
