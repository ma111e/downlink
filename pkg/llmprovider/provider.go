// Package llmprovider exposes a single Provider interface that every LLM
// backend in the project must satisfy. It ships a factory (New) that picks
// the right Eino model implementation for each supported provider type.
package llmprovider

import (
	"context"
	"fmt"
	"github.com/ma111e/downlink/pkg/claudeauth"
	"github.com/ma111e/downlink/pkg/codexauth"
	"net/http"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	log "github.com/sirupsen/logrus"

	einoclaude "github.com/cloudwego/eino-ext/components/model/claude"
	einoollama "github.com/cloudwego/eino-ext/components/model/ollama"
	einoopenai "github.com/cloudwego/eino-ext/components/model/openai"
)

// Provider is the minimal interface every LLM backend must satisfy.
type Provider interface {
	// Generate sends prompt to the model and returns the raw text response.
	Generate(ctx context.Context, prompt string) (string, error)
}

// Usage mirrors the subset of Eino's TokenUsage the monitoring layer persists.
// All fields are zero when the backend did not report usage (e.g. OAuth
// subscription providers), in which case GenerateWithUsage returns known=false.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// UsageGenerator is the optional add-on interface implemented by providers that
// can report token usage alongside the response. The gateway type-asserts for
// it; providers that don't implement it fall back to plain Generate.
type UsageGenerator interface {
	// GenerateWithUsage behaves like Generate but also returns token usage and
	// whether the backend actually reported it.
	GenerateWithUsage(ctx context.Context, prompt string) (string, Usage, bool, error)
}

// ChatModelProvider extends Provider with access to the underlying Eino ChatModel
// for use with ChatModelAgent.
type ChatModelProvider interface {
	Provider
	// ChatModel returns the underlying Eino BaseChatModel.
	ChatModel() model.BaseChatModel
}

// Config groups every knob the callers need to set.
type Config struct {
	ProviderType  string
	ProviderName  string // name of the provider config entry; used by openai-codex to find its pool
	ModelName     string
	BaseURL       string  // required for llamacpp / ollama; ignored for cloud providers
	APIKey        string  // optional for local providers
	Temperature   float64 // 0 means the backend default
	MaxTokens     int     // 0 means the backend default
	MaxRetries    int
	Timeout       time.Duration
	CodexManager  *codexauth.Manager  // required for openai-codex provider type
	ClaudeManager *claudeauth.Manager // required for claude-code provider type
}

// New builds the appropriate Provider for cfg.ProviderType.
func New(cfg Config) (ChatModelProvider, error) {
	switch cfg.ProviderType {
	case "llamacpp":
		if cfg.BaseURL == "" {
			return nil, fmt.Errorf("llamacpp provider requires a base_url")
		}
		return newEinoOpenAICompat(cfg, cfg.BaseURL)
	case "openai":
		return newEinoOpenAI(cfg)
	case "anthropic":
		return newEinoClaude(cfg)
	case "ollama":
		return newEinoOllama(cfg)
	case "mistral":
		return newEinoOpenAICompat(cfg, "https://api.mistral.ai/v1")
	case "openai-codex":
		return newCodexFromConfig(cfg)
	case "claude-code":
		return newClaudeCodeFromConfig(cfg)
	default:
		return newEinoOpenAI(cfg)
	}
}

func newCodexFromConfig(cfg Config) (*codexProvider, error) {
	if cfg.CodexManager == nil {
		return nil, fmt.Errorf("openai-codex provider requires CodexManager")
	}
	if cfg.ProviderName == "" {
		return nil, fmt.Errorf("openai-codex provider requires ProviderName")
	}
	pool := cfg.CodexManager.EnsurePool(cfg.ProviderName)
	return newCodexProviderFromPool(cfg.ModelName, cfg.BaseURL, pool, cfg.Timeout), nil
}

func newClaudeCodeFromConfig(cfg Config) (*claudeCodeProvider, error) {
	if cfg.ClaudeManager == nil {
		return nil, fmt.Errorf("claude-code provider requires ClaudeManager")
	}
	if cfg.ProviderName == "" {
		return nil, fmt.Errorf("claude-code provider requires ProviderName")
	}
	pool := cfg.ClaudeManager.EnsurePool(cfg.ProviderName)
	return newClaudeCodeProviderFromPool(cfg.ModelName, cfg.BaseURL, pool, cfg.MaxTokens, cfg.Timeout), nil
}

// ---------------------------------------------------------------------------
// Eino model wrapper (shared for all Eino-backed providers)
// ---------------------------------------------------------------------------

type einoProvider struct {
	cm model.BaseChatModel
}

func (p *einoProvider) ChatModel() model.BaseChatModel {
	return p.cm
}

func (p *einoProvider) Generate(ctx context.Context, prompt string) (string, error) {
	content, _, _, err := p.GenerateWithUsage(ctx, prompt)
	return content, err
}

// GenerateWithUsage implements UsageGenerator: it reads Eino's response usage
// metadata, which the plain Generate path discards.
func (p *einoProvider) GenerateWithUsage(ctx context.Context, prompt string) (string, Usage, bool, error) {
	msgs := []*schema.Message{
		{Role: schema.User, Content: prompt},
	}
	resp, err := p.cm.Generate(ctx, msgs)
	if err != nil {
		return "", Usage{}, false, err
	}
	usage, known := extractUsage(resp)
	return resp.Content, usage, known, nil
}

// extractUsage pulls token counts off an Eino message's ResponseMeta. known is
// false when the backend reported no usage (nil meta/usage).
func extractUsage(m *schema.Message) (Usage, bool) {
	if m == nil || m.ResponseMeta == nil || m.ResponseMeta.Usage == nil {
		return Usage{}, false
	}
	u := m.ResponseMeta.Usage
	return Usage{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		TotalTokens:      u.TotalTokens,
	}, true
}

// ---------------------------------------------------------------------------
// Eino OpenAI backend
// ---------------------------------------------------------------------------

func newEinoOpenAI(cfg Config) (*einoProvider, error) {
	ocfg := &einoopenai.ChatModelConfig{
		APIKey: cfg.APIKey,
		Model:  cfg.ModelName,
	}
	if cfg.BaseURL != "" {
		base := strings.TrimRight(cfg.BaseURL, "/")
		if !strings.HasSuffix(base, "/v1") {
			base += "/v1"
		}
		ocfg.BaseURL = base
	}
	if cfg.Temperature != 0 {
		t := float32(cfg.Temperature)
		ocfg.Temperature = &t
	}
	if cfg.MaxTokens != 0 {
		ocfg.MaxTokens = &cfg.MaxTokens
	}
	if cfg.Timeout != 0 {
		ocfg.Timeout = cfg.Timeout
	}

	log.Infof("OpenAI: initialising eino with BaseURL=%q, Model=%q, APIKey=%s", ocfg.BaseURL, ocfg.Model, "***")
	cm, err := einoopenai.NewChatModel(context.Background(), ocfg)
	if err != nil {
		return nil, fmt.Errorf("eino openai: %w", err)
	}
	return &einoProvider{cm: cm}, nil
}

// newEinoOpenAICompat creates an OpenAI-compatible provider with a custom base URL
// (used for Mistral and other OpenAI-compatible APIs).
func newEinoOpenAICompat(cfg Config, defaultBaseURL string) (*einoProvider, error) {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	ocfg := &einoopenai.ChatModelConfig{
		APIKey:  cfg.APIKey,
		Model:   cfg.ModelName,
		BaseURL: baseURL,
	}
	if cfg.Temperature != 0 {
		t := float32(cfg.Temperature)
		ocfg.Temperature = &t
	}
	if cfg.MaxTokens != 0 {
		ocfg.MaxTokens = &cfg.MaxTokens
	}
	if cfg.Timeout != 0 {
		ocfg.Timeout = cfg.Timeout
	}

	cm, err := einoopenai.NewChatModel(context.Background(), ocfg)
	if err != nil {
		return nil, fmt.Errorf("eino openai-compat (%s): %w", cfg.ProviderType, err)
	}
	return &einoProvider{cm: cm}, nil
}

// ---------------------------------------------------------------------------
// Eino Claude (Anthropic) backend
// ---------------------------------------------------------------------------

func newEinoClaude(cfg Config) (*einoProvider, error) {
	ccfg := &einoclaude.Config{
		APIKey:    cfg.APIKey,
		Model:     cfg.ModelName,
		MaxTokens: cfg.MaxTokens,
	}
	if cfg.BaseURL != "" {
		ccfg.BaseURL = &cfg.BaseURL
	}
	if cfg.Temperature != 0 {
		t := float32(cfg.Temperature)
		ccfg.Temperature = &t
	}
	if cfg.Timeout != 0 {
		ccfg.HTTPClient = &http.Client{Timeout: cfg.Timeout}
	}

	cm, err := einoclaude.NewChatModel(context.Background(), ccfg)
	if err != nil {
		return nil, fmt.Errorf("eino claude: %w", err)
	}
	return &einoProvider{cm: cm}, nil
}

// ---------------------------------------------------------------------------
// Eino Ollama backend
// ---------------------------------------------------------------------------

func newEinoOllama(cfg Config) (*einoProvider, error) {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	ocfg := &einoollama.ChatModelConfig{
		BaseURL: baseURL,
		Model:   cfg.ModelName,
	}
	if cfg.Timeout != 0 {
		ocfg.Timeout = cfg.Timeout
	}

	cm, err := einoollama.NewChatModel(context.Background(), ocfg)
	if err != nil {
		return nil, fmt.Errorf("eino ollama: %w", err)
	}
	return &einoProvider{cm: cm}, nil
}
