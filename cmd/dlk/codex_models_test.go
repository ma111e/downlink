package main

import (
	"os"
	"testing"
)

func TestGetCodexModelIDsLayeredFallback(t *testing.T) {
	// Clean up environment
	os.Unsetenv("OPENAI_ACCESS_TOKEN")
	os.Unsetenv("CHATGPT_TOKEN")
	os.Unsetenv("DOWNLINK_HOME")

	// Test Layer 4: Fallback to hardcoded defaults
	models := getCodexModelIDs("")
	if len(models) == 0 {
		t.Error("Expected hardcoded defaults, got empty list")
	}
	if models[0] != "gpt-4o" {
		t.Errorf("Expected first model to be gpt-4o, got %s", models[0])
	}
}

func TestAddForwardCompatModels(t *testing.T) {
	models := []string{"gpt-4", "gpt-3.5"}
	result := addForwardCompatModels(models)

	// Should include original models
	if len(result) < len(models) {
		t.Errorf("Forward compat should preserve original models, got %d expected at least %d", len(result), len(models))
	}

	// Should still contain original models
	for _, orig := range models {
		found := false
		for _, r := range result {
			if r == orig {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Original model %s not found in forward compat result", orig)
		}
	}
}

func TestDefaultCodexModels(t *testing.T) {
	if len(defaultCodexModels) == 0 {
		t.Error("Default Codex models list should not be empty")
	}

	expectedModels := []string{"gpt-4o", "gpt-4-turbo", "gpt-4", "gpt-3.5-turbo"}
	for i, expected := range expectedModels {
		if i >= len(defaultCodexModels) {
			t.Errorf("Expected at least %d models, got %d", len(expectedModels), len(defaultCodexModels))
			break
		}
		if defaultCodexModels[i] != expected {
			t.Errorf("Model at index %d: expected %s, got %s", i, expected, defaultCodexModels[i])
		}
	}
}

func TestCodexConfigPathResolution(t *testing.T) {
	// Test with custom DOWNLINK_HOME
	os.Setenv("DOWNLINK_HOME", "/tmp/test-downlink")
	configPath := getCodexConfigPath()
	expected := "/tmp/test-downlink/config.toml"
	if configPath != expected {
		t.Errorf("Expected config path %s, got %s", expected, configPath)
	}
	os.Unsetenv("DOWNLINK_HOME")

	// Test without DOWNLINK_HOME (should use ~/.downlink)
	home, _ := os.UserHomeDir()
	configPath = getCodexConfigPath()
	if configPath == "" {
		t.Error("Config path should not be empty")
	}
	if !contains(configPath, home) {
		t.Errorf("Config path should contain home directory, got %s", configPath)
	}
}

func TestCodexCachePathResolution(t *testing.T) {
	// Test with custom DOWNLINK_HOME
	os.Setenv("DOWNLINK_HOME", "/tmp/test-downlink")
	cachePath := getCodexCachePath()
	expected := "/tmp/test-downlink/models_cache.json"
	if cachePath != expected {
		t.Errorf("Expected cache path %s, got %s", expected, cachePath)
	}
	os.Unsetenv("DOWNLINK_HOME")
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
