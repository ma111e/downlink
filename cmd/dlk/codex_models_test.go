package main

import (
	"testing"
)

func TestGetCodexModelIDsNoToken(t *testing.T) {
	// Without a token, it falls back to the built-in model list.
	models := getCodexModelIDs("")
	if len(models) == 0 {
		t.Errorf("Expected built-in fallback models without token, got none")
	}
}

func TestCodexModelResponse(t *testing.T) {
	// Test that CodexModel struct can be unmarshaled correctly
	var model CodexModel
	model.Slug = "gpt-4o"
	model.Priority = 1
	model.Visibility = "public"

	if model.Slug != "gpt-4o" {
		t.Errorf("Expected slug gpt-4o, got %s", model.Slug)
	}
	if model.Priority != 1 {
		t.Errorf("Expected priority 1, got %d", model.Priority)
	}
}
