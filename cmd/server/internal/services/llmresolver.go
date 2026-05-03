package services

import (
	"downlink/cmd/server/internal/config"
	"downlink/pkg/llmprovider"
	"downlink/pkg/models"
	"fmt"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	defaultTemperature    = 0.3
	defaultMaxRetries     = 3
	defaultTimeout        = 20 * time.Minute
	defaultMaxTokensLarge = 200_000
)

// LLMRequest describes how to look up and configure a provider for a single call.
// Zero values mean "use the analysis config default".
type LLMRequest struct {
	// ProviderName selects an enabled provider by its config Name (highest priority).
	ProviderName string
	// ProviderType selects an enabled provider by type (used when ProviderName is empty).
	// When both are empty, the analysis config's named provider is used instead.
	ProviderType string
	ModelName    string // may be "" (use provider default) or "auto"

	// Per-call overrides; nil/zero means inherit from ProviderConfig, then fall
	// back to the package-level defaults above.
	Temperature *float64
	MaxRetries  *int
	Timeout     *time.Duration
	MaxTokens   int
}

// ResolvedLLM bundles a ready-to-use provider with the resolved identifiers.
type ResolvedLLM struct {
	Provider     llmprovider.ChatModelProvider
	ProviderType string
	ModelName    string
	Timeout      time.Duration
	MaxRetries   int
}

// ResolveLLM performs provider lookup, "auto" model resolution, default
// application, and provider construction in one place.
func ResolveLLM(req LLMRequest) (*ResolvedLLM, error) {
	// 1. Look up provider config.
	providerConfig, err := lookupProvider(req.ProviderName, req.ProviderType, req.ModelName)
	if err != nil {
		return nil, err
	}

	// 2. Resolve "auto" model name.
	if strings.EqualFold(providerConfig.ModelName, "auto") {
		available, fetchErr := fetchProviderModels(providerConfig.ProviderType, providerConfig)
		if fetchErr != nil || len(available) == 0 {
			return nil, fmt.Errorf("model_name is \"auto\" but could not list models for provider %s: %w", providerConfig.ProviderType, fetchErr)
		}
		providerConfig.ModelName = available[0].Id
		log.Infof("model_name \"auto\" resolved to %q for provider %s", providerConfig.ModelName, providerConfig.ProviderType)
	}

	// 3. Apply defaults: req override → ProviderConfig → package default.
	temperature := defaultTemperature
	if providerConfig.Temperature != nil {
		temperature = *providerConfig.Temperature
	}
	if req.Temperature != nil {
		temperature = *req.Temperature
	}

	maxRetries := defaultMaxRetries
	if providerConfig.MaxRetries != nil {
		maxRetries = *providerConfig.MaxRetries
	}
	if req.MaxRetries != nil {
		maxRetries = *req.MaxRetries
	}
	if maxRetries < 1 {
		maxRetries = 1
	}

	timeout := defaultTimeout
	if providerConfig.TimeoutMinutes != nil {
		timeout = time.Duration(*providerConfig.TimeoutMinutes) * time.Minute
	}
	if req.Timeout != nil {
		timeout = *req.Timeout
	}

	maxTokens := req.MaxTokens

	// 4. Build the provider.
	provider, err := llmprovider.New(llmprovider.Config{
		ProviderType: providerConfig.ProviderType,
		ProviderName: providerConfig.Name,
		ModelName:    providerConfig.ModelName,
		BaseURL:      providerConfig.BaseURL,
		APIKey:       providerConfig.APIKey,
		Temperature:  temperature,
		MaxTokens:    maxTokens,
		MaxRetries:   maxRetries,
		Timeout:      timeout,
		CodexManager: config.CodexManager,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize LLM client for %s: %w", providerConfig.ProviderType, err)
	}

	return &ResolvedLLM{
		Provider:     provider,
		ProviderType: providerConfig.ProviderType,
		ModelName:    providerConfig.ModelName,
		Timeout:      timeout,
		MaxRetries:   maxRetries,
	}, nil
}

// lookupProvider finds an enabled ProviderConfig.
// Priority: ProviderName (by config name) > ProviderType (by type) > analysis config default.
func lookupProvider(providerName, providerType, modelName string) (models.ProviderConfig, error) {
	if providerName != "" {
		return findEnabledProviderByName(providerName)
	}
	if providerType != "" {
		for _, p := range config.Config.Providers {
			if p.ProviderType == providerType && p.Enabled {
				if modelName != "" {
					p.ModelName = modelName
				}
				return p, nil
			}
		}
		return models.ProviderConfig{}, fmt.Errorf("provider %s is not enabled", providerType)
	}
	return findEnabledProviderByName(config.Config.Analysis.Provider)
}

// findEnabledProviderByName returns the ProviderConfig whose Name matches the
// given string and that is enabled. This is the single place that performs
// name-based provider lookup.
func findEnabledProviderByName(name string) (models.ProviderConfig, error) {
	if name == "" {
		return models.ProviderConfig{}, fmt.Errorf("no analysis provider configured")
	}
	for _, p := range config.Config.Providers {
		if p.Name == name && p.Enabled {
			return p, nil
		}
	}
	return models.ProviderConfig{}, fmt.Errorf("configured analysis provider %q not found or not enabled", name)
}
