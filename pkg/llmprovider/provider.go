// Package llmprovider exposes a single Provider interface that every LLM
// backend in the project must satisfy. It ships a factory (New) that picks
// the right Eino model implementation for each supported provider type.
package llmprovider

import (
	"context"
	"downlink/pkg/codexauth"
	"fmt"
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

// ChatModelProvider extends Provider with access to the underlying Eino ChatModel
// for use with ChatModelAgent.
type ChatModelProvider interface {
	Provider
	// ChatModel returns the underlying Eino BaseChatModel.
	ChatModel() model.BaseChatModel
}

// Config groups every knob the callers need to set.
type Config struct {
	ProviderType string
	ProviderName string // name of the provider config entry; used by openai-codex to find its pool
	ModelName    string
	BaseURL      string  // required for llamacpp / ollama; ignored for cloud providers
	APIKey       string  // optional for local providers
	Temperature  float64 // 0 means the backend default
	MaxTokens    int     // 0 means the backend default
	MaxRetries   int
	Timeout      time.Duration
	CodexManager *codexauth.Manager // required for openai-codex provider type
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
	msgs := []*schema.Message{
		{Role: schema.User, Content: prompt},
	}
	resp, err := p.cm.Generate(ctx, msgs)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
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
