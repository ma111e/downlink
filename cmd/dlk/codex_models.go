package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// defaultCodexModels is the hardcoded fallback list of Codex models
var defaultCodexModels = []string{
	"gpt-4o",
	"gpt-4-turbo",
	"gpt-4",
	"gpt-3.5-turbo",
}

// CodexModelResponse represents the OpenAI Codex API response
type CodexModelResponse struct {
	Models []CodexModel `json:"models"`
}

// CodexModel represents a single Codex model from the API
type CodexModel struct {
	Slug              string `json:"slug"`
	Priority          int    `json:"priority"`
	Visibility        string `json:"visibility"`
	SupportedInAPI    bool   `json:"supported_in_api"`
	DisplayName       string `json:"display_name"`
	Description       string `json:"description"`
	ContextWindowSize int    `json:"context_window_size"`
}

// getCodexModelIDs fetches available Codex models using a layered fallback strategy.
// Layer 1: Live API fetch (requires OAuth token)
// Layer 2: Local config file
// Layer 3: Local models cache
// Layer 4: Hardcoded defaults
func getCodexModelIDs(accessToken string) []string {
	// Layer 1: Try live API fetch if we have a token
	if accessToken != "" {
		if models, err := fetchCodexModelsFromAPI(accessToken); err == nil && len(models) > 0 {
			return models
		}
	}

	// Layer 2: Try config file
	if models := fetchCodexModelsFromConfig(); len(models) > 0 {
		return models
	}

	// Layer 3: Try cache file
	if models := fetchCodexModelsFromCache(); len(models) > 0 {
		return models
	}

	// Layer 4: Fall back to hardcoded defaults
	return defaultCodexModels
}

// fetchCodexModelsFromAPI calls OpenAI's Codex models endpoint
func fetchCodexModelsFromAPI(accessToken string) ([]string, error) {
	req, err := http.NewRequest("GET", "https://chatgpt.com/backend-api/codex/models?client_version=1.0.0", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", "downlink-cli/1.0")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var apiResp CodexModelResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, err
	}

	// Filter and sort models
	var models []CodexModel
	for _, m := range apiResp.Models {
		// Skip hidden models
		if m.Visibility == "hide" || m.Visibility == "hidden" {
			continue
		}
		// Note: We do NOT filter on SupportedInAPI because that indicates
		// public API availability, not OAuth-backed Codex availability
		models = append(models, m)
	}

	// Sort by priority (lower priority = appears first)
	slices.SortFunc(models, func(a, b CodexModel) int {
		if a.Priority != b.Priority {
			return a.Priority - b.Priority
		}
		return strings.Compare(a.Slug, b.Slug)
	})

	// Extract slugs and apply forward-compat synthesis
	slugs := make([]string, len(models))
	for i, m := range models {
		slugs[i] = m.Slug
	}

	return addForwardCompatModels(slugs), nil
}

// fetchCodexModelsFromConfig reads the local config file
func fetchCodexModelsFromConfig() []string {
	configPath := getCodexConfigPath()
	if configPath == "" {
		return nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}

	// Simple TOML-like parsing for the model key
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "model") && strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				model := strings.TrimSpace(parts[1])
				model = strings.Trim(model, `"'`)
				if model != "" {
					return []string{model}
				}
			}
		}
	}

	return nil
}

// fetchCodexModelsFromCache reads the local models cache file
func fetchCodexModelsFromCache() []string {
	cachePath := getCodexCachePath()
	if cachePath == "" {
		return nil
	}

	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil
	}

	var cached struct {
		Models []CodexModel `json:"models"`
	}

	if err := json.Unmarshal(data, &cached); err != nil {
		return nil
	}

	// Filter and sort models (same logic as API)
	var models []CodexModel
	for _, m := range cached.Models {
		if m.Visibility == "hide" || m.Visibility == "hidden" {
			continue
		}
		models = append(models, m)
	}

	slices.SortFunc(models, func(a, b CodexModel) int {
		if a.Priority != b.Priority {
			return a.Priority - b.Priority
		}
		return strings.Compare(a.Slug, b.Slug)
	})

	slugs := make([]string, len(models))
	for i, m := range models {
		slugs[i] = m.Slug
	}

	return addForwardCompatModels(slugs)
}

// addForwardCompatModels synthesizes newer model versions if older ones are present
func addForwardCompatModels(models []string) []string {
	// Map of older models to their forward-compat newer versions
	// Example: gpt-4 → gpt-4.5 (if a template exists)
	forwardCompat := map[string][]string{
		// Add mappings as needed based on known template models
		// e.g., "gpt-4": {"gpt-4.5"},
	}

	result := slices.Clone(models)
	for _, model := range models {
		if newVersions, ok := forwardCompat[model]; ok {
			for _, newVer := range newVersions {
				if !slices.Contains(result, newVer) {
					result = append(result, newVer)
				}
			}
		}
	}

	return result
}

// getCodexConfigPath returns the path to ~/.downlink/config.toml or $DOWNLINK_HOME/config.toml
func getCodexConfigPath() string {
	if home := os.Getenv("DOWNLINK_HOME"); home != "" {
		return filepath.Join(home, "config.toml")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(home, ".downlink", "config.toml")
}

// getCodexCachePath returns the path to ~/.downlink/models_cache.json or $DOWNLINK_HOME/models_cache.json
func getCodexCachePath() string {
	if home := os.Getenv("DOWNLINK_HOME"); home != "" {
		return filepath.Join(home, "models_cache.json")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(home, ".downlink", "models_cache.json")
}

// saveModelsCacheFile writes models to the local cache for future offline use
func saveModelsCacheFile(models []CodexModel) error {
	cachePath := getCodexCachePath()
	if cachePath == "" {
		return fmt.Errorf("could not determine cache path")
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o700); err != nil {
		return err
	}

	data := map[string]interface{}{
		"models": models,
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(cachePath, jsonData, 0o600)
}
