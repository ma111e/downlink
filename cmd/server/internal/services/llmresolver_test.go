package services

import (
	"strings"
	"testing"

	"github.com/ma111e/downlink/pkg/models"
)

func TestResolveStepProvider(t *testing.T) {
	overrides := map[string]models.StepProviderOverride{
		"summary":  {Provider: "prov-b", Model: "model-b"},
		"partial":  {Provider: "prov-c"}, // model empty -> keep default model
		"modelonly": {Model: "model-d"},  // provider empty -> keep default provider
	}

	t.Run("no override returns defaults", func(t *testing.T) {
		p, m := resolveStepProvider("categorize", "prov-a", "model-a", overrides)
		if p != "prov-a" || m != "model-a" {
			t.Fatalf("got (%q,%q), want defaults (prov-a, model-a)", p, m)
		}
	})
	t.Run("full override wins", func(t *testing.T) {
		p, m := resolveStepProvider("summary", "prov-a", "model-a", overrides)
		if p != "prov-b" || m != "model-b" {
			t.Fatalf("got (%q,%q), want (prov-b, model-b)", p, m)
		}
	})
	t.Run("partial override keeps default model", func(t *testing.T) {
		p, m := resolveStepProvider("partial", "prov-a", "model-a", overrides)
		if p != "prov-c" || m != "model-a" {
			t.Fatalf("got (%q,%q), want (prov-c, model-a)", p, m)
		}
	})
	t.Run("model-only override keeps default provider", func(t *testing.T) {
		p, m := resolveStepProvider("modelonly", "prov-a", "model-a", overrides)
		if p != "prov-a" || m != "model-d" {
			t.Fatalf("got (%q,%q), want (prov-a, model-d)", p, m)
		}
	})
}

func TestDecideModelOnlyProvider(t *testing.T) {
	t.Run("none is error", func(t *testing.T) {
		_, err := decideModelOnlyProvider("gpt-x", nil)
		if err == nil || !strings.Contains(err.Error(), "not available") {
			t.Fatalf("err = %v, want 'not available'", err)
		}
	})
	t.Run("exactly one", func(t *testing.T) {
		got, err := decideModelOnlyProvider("gpt-x", []models.ProviderConfig{{Name: "solo"}})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got.Name != "solo" {
			t.Fatalf("got %q, want solo", got.Name)
		}
	})
	t.Run("duplicate names collapse to one", func(t *testing.T) {
		got, err := decideModelOnlyProvider("gpt-x", []models.ProviderConfig{{Name: "dup"}, {Name: "dup"}})
		if err != nil {
			t.Fatalf("err = %v, want single provider after dedupe", err)
		}
		if got.Name != "dup" {
			t.Fatalf("got %q, want dup", got.Name)
		}
	})
	t.Run("multiple distinct is ambiguity error", func(t *testing.T) {
		_, err := decideModelOnlyProvider("gpt-x", []models.ProviderConfig{{Name: "a"}, {Name: "b"}})
		if err == nil || !strings.Contains(err.Error(), "multiple providers") {
			t.Fatalf("err = %v, want ambiguity error naming multiple providers", err)
		}
	})
}

func TestFindEnabledProviderByName(t *testing.T) {
	withGlobalConfig(t, &models.ServerConfig{Providers: []models.ProviderConfig{
		{Name: "enabled-prov", ProviderType: "openai", Enabled: true},
		{Name: "disabled-prov", ProviderType: "ollama", Enabled: false},
	}})

	t.Run("empty name", func(t *testing.T) {
		if _, err := findEnabledProviderByName(""); err == nil {
			t.Fatal("empty name err = nil, want error")
		}
	})
	t.Run("enabled match", func(t *testing.T) {
		got, err := findEnabledProviderByName("enabled-prov")
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got.ProviderType != "openai" {
			t.Fatalf("got type %q, want openai", got.ProviderType)
		}
	})
	t.Run("disabled provider is not found", func(t *testing.T) {
		if _, err := findEnabledProviderByName("disabled-prov"); err == nil {
			t.Fatal("disabled provider err = nil, want not-found/disabled error")
		}
	})
	t.Run("unknown name", func(t *testing.T) {
		if _, err := findEnabledProviderByName("ghost"); err == nil {
			t.Fatal("unknown name err = nil, want error")
		}
	})
}

func TestResolveProviderSelectionByProviderToken(t *testing.T) {
	withGlobalConfig(t, &models.ServerConfig{Providers: []models.ProviderConfig{
		{Name: "my-openai", ProviderType: "openai", ModelName: "gpt-4", Enabled: true},
		{Name: "ollama", ProviderType: "ollama", Enabled: true}, // Name == another's Type below
		{Name: "openai", ProviderType: "custom", Enabled: true}, // Name collides with a type token
	}})

	t.Run("resolves by profile name with model override", func(t *testing.T) {
		got, err := resolveProviderSelection("my-openai", "gpt-4o")
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got.Name != "my-openai" || got.ModelName != "gpt-4o" {
			t.Fatalf("got %+v, want my-openai/gpt-4o", got)
		}
	})

	t.Run("ambiguous name-and-type is an error", func(t *testing.T) {
		// "openai" is both a config Name (custom provider) and a ProviderType (my-openai).
		_, err := resolveProviderSelection("openai", "")
		if err == nil || !strings.Contains(err.Error(), "ambiguous") {
			t.Fatalf("err = %v, want ambiguity error", err)
		}
	})

	t.Run("unknown token is an error", func(t *testing.T) {
		_, err := resolveProviderSelection("nonesuch", "")
		if err == nil || !strings.Contains(err.Error(), "no enabled provider") {
			t.Fatalf("err = %v, want no-match error", err)
		}
	})
}

func TestLookupProviderPriority(t *testing.T) {
	withGlobalConfig(t, &models.ServerConfig{
		Analysis: models.AnalysisConfig{Provider: "analysis-default"},
		Providers: []models.ProviderConfig{
			{Name: "analysis-default", ProviderType: "openai", Enabled: true},
			{Name: "by-name", ProviderType: "ollama", Enabled: true},
			{Name: "typed", ProviderType: "anthropic", Enabled: true},
		},
	})

	t.Run("ProviderName has top priority", func(t *testing.T) {
		got, err := lookupProvider("by-name", "anthropic", "", "")
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got.Name != "by-name" {
			t.Fatalf("got %q, want by-name (ProviderName wins over ProviderType)", got.Name)
		}
	})
	t.Run("ProviderType with model override", func(t *testing.T) {
		got, err := lookupProvider("", "anthropic", "", "claude-x")
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got.Name != "typed" || got.ModelName != "claude-x" {
			t.Fatalf("got %+v, want typed/claude-x", got)
		}
	})
	t.Run("ProviderType not enabled", func(t *testing.T) {
		_, err := lookupProvider("", "nonexistent-type", "", "")
		if err == nil || !strings.Contains(err.Error(), "not enabled") {
			t.Fatalf("err = %v, want not-enabled error", err)
		}
	})
	t.Run("falls back to analysis config default", func(t *testing.T) {
		got, err := lookupProvider("", "", "", "")
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got.Name != "analysis-default" {
			t.Fatalf("got %q, want analysis-default fallback", got.Name)
		}
	})
}
