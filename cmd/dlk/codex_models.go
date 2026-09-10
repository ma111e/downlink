package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"
)

// codexModelsURL is the Codex CLI's own model catalog. It is public, needs no
// credentials, and stays in sync with what Codex ships.
const codexModelsURL = "https://raw.githubusercontent.com/openai/codex/refs/heads/main/codex-rs/models-manager/models.json"

// CodexModelResponse represents the Codex models.json catalog
type CodexModelResponse struct {
	Models []CodexModel `json:"models"`
}

// CodexModel represents a single Codex model from the catalog
type CodexModel struct {
	Slug           string `json:"slug"`
	Priority       int    `json:"priority"`
	Visibility     string `json:"visibility"`
	SupportedInAPI bool   `json:"supported_in_api"`
	DisplayName    string `json:"display_name"`
	Description    string `json:"description"`
	ContextWindow  int    `json:"context_window"`
}

// fallbackCodexModels lists the Codex-available model slugs newest first, used
// when the catalog cannot be fetched. Mirrors the entries marked visible in
// upstream models.json.
var fallbackCodexModels = []string{
	"gpt-6-astra",
	"gpt-5.6-sol",
	"gpt-5.6-terra",
	"gpt-5.6-luna",
	"gpt-5.5",
}

// getCodexModelIDs returns the available Codex models sorted by priority, with
// hidden models filtered out. Falls back to a hardcoded list if the catalog
// cannot be fetched.
func getCodexModelIDs() []string {
	models, err := fetchCodexModels()
	if err != nil || len(models) == 0 {
		fmt.Println("Note: using built-in model list (catalog fetch failed)")
		return fallbackCodexModels
	}

	return models
}

// fetchCodexModels reads the Codex model catalog published in the codex repo
func fetchCodexModels() ([]string, error) {
	req, err := http.NewRequest("GET", codexModelsURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "downlink-cli/1.0")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("catalog returned status %d", resp.StatusCode)
	}

	var catalog CodexModelResponse
	if err := json.NewDecoder(resp.Body).Decode(&catalog); err != nil {
		return nil, err
	}

	return codexModelSlugs(catalog.Models), nil
}

// codexModelSlugs filters out hidden models and returns the remaining slugs
// ordered by priority.
func codexModelSlugs(all []CodexModel) []string {
	var models []CodexModel
	for _, m := range all {
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

	slugs := make([]string, len(models))
	for i, m := range models {
		slugs[i] = m.Slug
	}

	return slugs
}
