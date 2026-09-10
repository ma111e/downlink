package main

import (
	"testing"
)

func TestFallbackCodexModelsNotEmpty(t *testing.T) {
	if len(fallbackCodexModels) == 0 {
		t.Error("Expected a non-empty built-in fallback model list")
	}
}

func TestCodexModelSlugsFiltersAndSorts(t *testing.T) {
	slugs := codexModelSlugs([]CodexModel{
		{Slug: "gpt-5.5", Priority: 12, Visibility: "list"},
		{Slug: "gpt-daybreak-red-latest", Priority: 11, Visibility: "hide"},
		{Slug: "gpt-6-astra", Priority: 1, Visibility: "list"},
		{Slug: "gpt-legacy", Priority: 20, Visibility: "hidden"},
	})

	want := []string{"gpt-6-astra", "gpt-5.5"}
	if len(slugs) != len(want) {
		t.Fatalf("Expected %v, got %v", want, slugs)
	}
	for i := range want {
		if slugs[i] != want[i] {
			t.Errorf("Expected %v, got %v", want, slugs)
			break
		}
	}
}

func TestCodexModelSlugsTiesBreakBySlug(t *testing.T) {
	slugs := codexModelSlugs([]CodexModel{
		{Slug: "b-model", Priority: 5, Visibility: "list"},
		{Slug: "a-model", Priority: 5, Visibility: "list"},
	})

	if slugs[0] != "a-model" || slugs[1] != "b-model" {
		t.Errorf("Expected slug-ordered tie break, got %v", slugs)
	}
}
