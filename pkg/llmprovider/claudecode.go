package llmprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ma111e/downlink/pkg/claudeauth"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// defaultClaudeMaxTokens is used when the caller does not set a max_tokens.
// The Anthropic Messages API requires max_tokens on every request.
const defaultClaudeMaxTokens = 8192

// maxClaudeOutputTokens caps the requested max_tokens. Anthropic's max_tokens is
// an output-token ceiling (well below the context window); callers such as the
// digest/analysis paths pass defaultMaxTokensLarge (200000), which the API would
// reject with HTTP 400. 32000 is safe across current Opus/Sonnet/Haiku models
// without output-extension betas and is ample for digest/analysis output.
const maxClaudeOutputTokens = 32000

// claudeCodeProvider calls the Anthropic Messages API with a Claude Code
// subscription OAuth token, using a claudeauth.Pool for token management and
// credential rotation.
type claudeCodeProvider struct {
	modelName string
	pool      *claudeauth.Pool
	baseURL   string
	maxTokens int
	timeout   time.Duration
}

func newClaudeCodeProviderFromPool(modelName, baseURL string, pool *claudeauth.Pool, maxTokens int, timeout time.Duration) *claudeCodeProvider {
	if baseURL == "" {
		baseURL = claudeauth.APIBaseURL
	}
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	if maxTokens <= 0 {
		maxTokens = defaultClaudeMaxTokens
	}
	if maxTokens > maxClaudeOutputTokens {
		maxTokens = maxClaudeOutputTokens
	}
	return &claudeCodeProvider{
		modelName: modelName,
		pool:      pool,
		baseURL:   strings.TrimRight(baseURL, "/"),
		maxTokens: maxTokens,
		timeout:   timeout,
	}
}

// Generate implements Provider.
func (p *claudeCodeProvider) Generate(ctx context.Context, prompt string) (string, error) {
	msgs := []*schema.Message{{Role: schema.User, Content: prompt}}
	resp, err := p.generateMessages(ctx, msgs)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// ChatModel implements ChatModelProvider.
func (p *claudeCodeProvider) ChatModel() model.BaseChatModel {
	return &claudeCodeChatModel{provider: p}
}

// generateMessages is the shared implementation for both Generate and the
// BaseChatModel adapter, with the same acquire / 401-refresh / 429-rotate
// retry structure as the codex provider.
func (p *claudeCodeProvider) generateMessages(ctx context.Context, msgs []*schema.Message) (*schema.Message, error) {
	lease, err := p.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}

	resp, err := p.callAPI(ctx, lease, msgs)
	if err == nil {
		lease.MarkOK()
		return resp, nil
	}

	// On usage-limit (quota exhausted): mark this credential rate-limited until
	// the reported reset and propagate immediately. No rotation, no retry — the
	// caller aborts the whole run rather than hammering flagged accounts.
	if errors.Is(err, ErrUsageLimitReached) {
		var ule *usageLimitError
		if errors.As(err, &ule) {
			lease.MarkRateLimited(ule.resetAt)
		}
		return nil, err
	}

	if isClaudeAuthError(err) {
		lease.MarkAuthFailed(err.Error())
		retryLease, err2 := p.pool.Acquire(ctx)
		if err2 != nil {
			return nil, fmt.Errorf("claude-code: auth failed and no healthy credential for retry: %w", err)
		}
		resp, err2 = p.callAPI(ctx, retryLease, msgs)
		if err2 != nil {
			retryLease.MarkAuthFailed(err2.Error())
			return nil, fmt.Errorf("%w: %s", claudeauth.ErrReloginRequired, err2.Error())
		}
		retryLease.MarkOK()
		return resp, nil
	}

	if isClaudeRateLimit(err) {
		resetAt := time.Now().Add(time.Hour)
		if rl, ok := err.(*claudeRateLimitError); ok {
			resetAt = rl.resetAt
		}
		lease.MarkRateLimited(resetAt)
		retryLease, err2 := p.pool.Acquire(ctx)
		if err2 != nil {
			return nil, fmt.Errorf("claude-code: rate limited and no other credential available: %w", err)
		}
		resp, err2 = p.callAPI(ctx, retryLease, msgs)
		if err2 != nil {
			if isClaudeRateLimit(err2) {
				retryLease.MarkRateLimited(resetAt)
			}
			return nil, err2
		}
		retryLease.MarkOK()
		return resp, nil
	}

	return nil, err
}

// messagesRequest is the body sent to the Anthropic Messages API.
type messagesRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	System    []systemBlock   `json:"system,omitempty"`
	Messages  []claudeMessage `json:"messages"`
}

type systemBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type claudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// messagesResponse is the (non-streaming) Anthropic Messages API response.
type messagesResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func (p *claudeCodeProvider) callAPI(ctx context.Context, lease *claudeauth.Lease, msgs []*schema.Message) (*schema.Message, error) {
	// The first system block must carry the Claude Code identity; any real
	// system prompt is appended after it.
	system := []systemBlock{{Type: "text", Text: claudeauth.ClaudeCodeSystemPrefix}}
	var turns []claudeMessage
	for _, m := range msgs {
		switch m.Role {
		case schema.System:
			if strings.TrimSpace(m.Content) != "" {
				system = append(system, systemBlock{Type: "text", Text: m.Content})
			}
		case schema.Assistant:
			turns = append(turns, claudeMessage{Role: "assistant", Content: m.Content})
		default:
			turns = append(turns, claudeMessage{Role: "user", Content: m.Content})
		}
	}

	body, err := json.Marshal(messagesRequest{
		Model:     p.modelName,
		MaxTokens: p.maxTokens,
		System:    system,
		Messages:  turns,
	})
	if err != nil {
		return nil, err
	}

	reqCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, p.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
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

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		raw, _ := io.ReadAll(resp.Body)
		return nil, &claudeAPIError{statusCode: resp.StatusCode, body: string(raw)}
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		raw, _ := io.ReadAll(resp.Body)
		if ule, ok := parseUsageLimit("claude-code", string(raw)); ok {
			return nil, ule
		}
		ra := parseClaudeRetryAfter(resp.Header.Get("Retry-After"))
		return nil, &claudeRateLimitError{resetAt: ra, body: string(raw)}
	}
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		if ule, ok := parseUsageLimit("claude-code", string(raw)); ok {
			return nil, ule
		}
		return nil, fmt.Errorf("claude-code API error: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("claude-code: read response: %w", err)
	}
	var parsed messagesResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("claude-code: decode response: %w", err)
	}
	var text strings.Builder
	for _, c := range parsed.Content {
		if c.Type == "text" {
			text.WriteString(c.Text)
		}
	}
	msg := &schema.Message{Role: schema.Assistant, Content: text.String()}
	if parsed.Usage.InputTokens > 0 || parsed.Usage.OutputTokens > 0 {
		msg.ResponseMeta = &schema.ResponseMeta{Usage: &schema.TokenUsage{
			PromptTokens:     parsed.Usage.InputTokens,
			CompletionTokens: parsed.Usage.OutputTokens,
			TotalTokens:      parsed.Usage.InputTokens + parsed.Usage.OutputTokens,
		}}
	}
	return msg, nil
}

// GenerateWithUsage implements UsageGenerator so the gateway can record token
// usage for claude-code subscription calls.
func (p *claudeCodeProvider) GenerateWithUsage(ctx context.Context, prompt string) (string, Usage, bool, error) {
	resp, err := p.generateMessages(ctx, []*schema.Message{{Role: schema.User, Content: prompt}})
	if err != nil {
		return "", Usage{}, false, err
	}
	u, known := extractUsage(resp)
	return resp.Content, u, known, nil
}

// ---------------------------------------------------------------------------
// Error types for routing logic
// ---------------------------------------------------------------------------

type claudeAPIError struct {
	statusCode int
	body       string
}

func (e *claudeAPIError) Error() string {
	return fmt.Sprintf("claude-code: HTTP %d: %s", e.statusCode, e.body)
}

type claudeRateLimitError struct {
	resetAt time.Time
	body    string
}

func (e *claudeRateLimitError) Error() string {
	return fmt.Sprintf("claude-code: rate limited (reset %s): %s", e.resetAt.Format(time.RFC3339), e.body)
}

func isClaudeAuthError(err error) bool {
	if e, ok := err.(*claudeAPIError); ok {
		return e.statusCode == http.StatusUnauthorized || e.statusCode == http.StatusForbidden
	}
	return false
}

func isClaudeRateLimit(err error) bool {
	_, ok := err.(*claudeRateLimitError)
	return ok
}

func parseClaudeRetryAfter(header string) time.Time {
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

type claudeCodeChatModel struct {
	provider *claudeCodeProvider
}

func (m *claudeCodeChatModel) Generate(ctx context.Context, msgs []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return m.provider.generateMessages(ctx, msgs)
}

func (m *claudeCodeChatModel) Stream(ctx context.Context, msgs []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	// Streaming not implemented; fall back to non-streaming like the codex provider.
	msg, err := m.provider.generateMessages(ctx, msgs)
	if err != nil {
		return nil, err
	}
	sr, sw := schema.Pipe[*schema.Message](1)
	sw.Send(msg, nil)
	sw.Close()
	return sr, nil
}
