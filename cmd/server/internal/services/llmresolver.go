package services

import (
	"fmt"
	"github.com/ma111e/downlink/cmd/server/internal/config"
	"github.com/ma111e/downlink/pkg/llmprovider"
	"github.com/ma111e/downlink/pkg/models"
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
	ProviderType string
	// Provider is the auto-detect token: it may be a provider type OR a config
	// profile name; the server figures out which. Used by the user-facing
	// --provider flag. Lower priority than the explicit ProviderName/ProviderType
	// fields above (which the internal profile/queue callers set directly).
	Provider  string
	ModelName string // may be "" (use provider default) or "auto"

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
	providerConfig, err := lookupProvider(req.ProviderName, req.ProviderType, req.Provider, req.ModelName)
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
		ProviderType:  providerConfig.ProviderType,
		ProviderName:  providerConfig.Name,
		ModelName:     providerConfig.ModelName,
		BaseURL:       providerConfig.BaseURL,
		APIKey:        providerConfig.APIKey,
		Temperature:   temperature,
		MaxTokens:     maxTokens,
		MaxRetries:    maxRetries,
		Timeout:       timeout,
		CodexManager:  config.CodexManager,
		ClaudeManager: config.ClaudeManager,
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
// Priority: ProviderName (explicit config name) > ProviderType (explicit type) >
// provider (auto-detect token, type-or-name) > model-only search > analysis config default.
func lookupProvider(providerName, providerType, provider, modelName string) (models.ProviderConfig, error) {
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
	// User-facing --provider / --model: the server figures out the match.
	if provider != "" || modelName != "" {
		return resolveProviderSelection(provider, modelName)
	}
	return findEnabledProviderByName(config.Config.Analysis.Provider)
}

// resolveProviderSelection implements the user-facing --provider/--model resolution:
//   - provider given: auto-detect whether it is a provider type or a config profile
//     name (error if it is unambiguously neither, or ambiguously both).
//   - provider empty, model given: search every enabled provider's live model list
//     and pick the unique provider that offers the model (error if none or several).
func resolveProviderSelection(provider, model string) (models.ProviderConfig, error) {
	if provider != "" {
		var byName *models.ProviderConfig
		var typeMatch *models.ProviderConfig
		for i := range config.Config.Providers {
			p := config.Config.Providers[i]
			if !p.Enabled {
				continue
			}
			if p.Name == provider && byName == nil {
				cp := p
				byName = &cp
			}
			if p.ProviderType == provider && typeMatch == nil {
				cp := p
				typeMatch = &cp
			}
		}
		if byName != nil && typeMatch != nil {
			return models.ProviderConfig{}, fmt.Errorf("%q is ambiguous: it is both a provider type and a profile name; use --profile to select the profile, or rename it", provider)
		}
		chosen := byName
		if chosen == nil {
			chosen = typeMatch
		}
		if chosen == nil {
			return models.ProviderConfig{}, fmt.Errorf("no enabled provider matches %q (not a known provider type or profile name)", provider)
		}
		cfg := *chosen
		if model != "" {
			cfg.ModelName = model
		}
		return cfg, nil
	}

	// Model-only: find which enabled provider(s) offer this model.
	candidates := findProvidersOfferingModel(model)
	chosen, err := decideModelOnlyProvider(model, candidates)
	if err != nil {
		return models.ProviderConfig{}, err
	}
	chosen.ModelName = model
	return chosen, nil
}

// findProvidersOfferingModel returns the enabled provider configs whose live model
// list includes a model whose Id matches the given name (case-insensitive). Live
// fetches are cached by providerType+":"+baseURL so providers sharing an endpoint
// are queried only once. Providers whose models cannot be fetched are skipped.
func findProvidersOfferingModel(model string) []models.ProviderConfig {
	type listResult struct {
		offers bool
	}
	cache := make(map[string]listResult)
	var candidates []models.ProviderConfig
	for _, p := range config.Config.Providers {
		if !p.Enabled {
			continue
		}
		key := p.ProviderType + ":" + p.BaseURL
		res, ok := cache[key]
		if !ok {
			available, err := fetchProviderModels(p.ProviderType, p)
			if err != nil {
				log.WithError(err).Warnf("model-only resolution: could not list models for provider %q (%s), skipping", p.Name, p.ProviderType)
				cache[key] = listResult{offers: false}
				continue
			}
			offers := false
			for _, m := range available {
				if strings.EqualFold(m.Id, model) {
					offers = true
					break
				}
			}
			res = listResult{offers: offers}
			cache[key] = res
		}
		if res.offers {
			candidates = append(candidates, p)
		}
	}
	return candidates
}

// decideModelOnlyProvider resolves a model-only selection from the candidate list.
// Pure decision logic (no network), reduced to distinct provider Names:
// none → error, exactly one → that provider, several → ambiguity error.
func decideModelOnlyProvider(model string, candidates []models.ProviderConfig) (models.ProviderConfig, error) {
	seen := make(map[string]bool)
	var distinct []models.ProviderConfig
	for _, c := range candidates {
		if seen[c.Name] {
			continue
		}
		seen[c.Name] = true
		distinct = append(distinct, c)
	}
	switch len(distinct) {
	case 0:
		return models.ProviderConfig{}, fmt.Errorf("model %q is not available from any enabled provider", model)
	case 1:
		return distinct[0], nil
	default:
		names := make([]string, len(distinct))
		for i, c := range distinct {
			names[i] = c.Name
		}
		return models.ProviderConfig{}, fmt.Errorf("model %q is available from multiple providers (%s); specify --provider to disambiguate", model, strings.Join(names, ", "))
	}
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
