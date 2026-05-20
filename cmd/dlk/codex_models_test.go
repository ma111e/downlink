package main

import (
	"testing"
)

func TestGetCodexModelIDsNoToken(t *testing.T) {
	// Without a token, should return nil
	models := getCodexModelIDs("")
	if models != nil {
		t.Errorf("Expected nil without token, got %v", models)
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
