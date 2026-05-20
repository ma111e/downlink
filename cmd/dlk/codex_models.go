package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
)

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

// getCodexModelIDs fetches available Codex models directly from the OpenAI Codex API.
// Returns models sorted by priority, with hidden models filtered out.
// Returns empty list if token is unavailable or API call fails.
func getCodexModelIDs(accessToken string) []string {
	if accessToken == "" {
		return nil
	}

	models, err := fetchCodexModelsFromAPI(accessToken)
	if err != nil {
		return nil
	}

	return models
}

// fetchCodexModelsFromAPI calls OpenAI's Codex models endpoint directly
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

	// Extract slugs
	slugs := make([]string, len(models))
	for i, m := range models {
		slugs[i] = m.Slug
	}

	return slugs, nil
}
