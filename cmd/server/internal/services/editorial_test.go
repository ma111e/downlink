package services

import (
	"testing"

	"github.com/ma111e/downlink/cmd/server/internal/config"
	"github.com/ma111e/downlink/pkg/models"
	"github.com/ma111e/downlink/pkg/protos"
	"github.com/ma111e/downlink/pkg/scoring"
)

// withGlobalConfig sets config.Config for the test and restores it afterwards.
func withGlobalConfig(t *testing.T, cfg *models.ServerConfig) {
	t.Helper()
	orig := config.Config
	config.Config = cfg
	t.Cleanup(func() { config.Config = orig })
}

func boolPtr(b bool) *bool { return &b }

func TestResolveEditorialInheritsGlobalWhenProfileEmpty(t *testing.T) {
	withGlobalConfig(t, &models.ServerConfig{Analysis: models.AnalysisConfig{
		Provider:     "global-prov",
		Persona:      "global-persona",
		WritingStyle: "global-style",
		VibeScore:    true,
		Glossary:     true,
	}})

	eff := ResolveEditorial("default", nil)
	if eff.ProfileId != "default" {
		t.Errorf("ProfileId = %q, want default", eff.ProfileId)
	}
	if eff.Provider != "global-prov" || eff.Persona != "global-persona" || eff.WritingStyle != "global-style" {
		t.Errorf("did not inherit global scalars: %+v", eff)
	}
	if !eff.VibeScore || !eff.Glossary {
		t.Errorf("did not inherit global bools: vibe=%v glossary=%v", eff.VibeScore, eff.Glossary)
	}
	// With no rubric override, scoring must equal the package default.
	if eff.Scoring != scoring.DefaultConfig() {
		t.Errorf("Scoring = %+v, want DefaultConfig", eff.Scoring)
	}
}

func TestResolveEditorialProfileOverridesGlobal(t *testing.T) {
	withGlobalConfig(t, &models.ServerConfig{Analysis: models.AnalysisConfig{
		Provider:  "global-prov",
		Persona:   "global-persona",
		VibeScore: true,  // profile will turn this OFF via a non-nil pointer
		Glossary:  false, // profile will turn this ON
	}})

	ed := &models.ProfileEditorial{
		Provider:  "profile-prov",
		Persona:   "", // empty: must inherit global-persona
		VibeScore: boolPtr(false),
		Glossary:  boolPtr(true),
	}
	eff := ResolveEditorial("infosec", ed)

	if eff.Provider != "profile-prov" {
		t.Errorf("Provider = %q, want profile override", eff.Provider)
	}
	if eff.Persona != "global-persona" {
		t.Errorf("Persona = %q, want inherited global-persona (empty profile field)", eff.Persona)
	}
	if eff.VibeScore != false {
		t.Errorf("VibeScore = %v, want false (profile pointer overrides global true)", eff.VibeScore)
	}
	if eff.Glossary != true {
		t.Errorf("Glossary = %v, want true (profile pointer overrides global false)", eff.Glossary)
	}
}

func TestResolveEditorialNilGlobalConfigIsSafe(t *testing.T) {
	withGlobalConfig(t, nil)
	eff := ResolveEditorial("p", nil)
	if eff.Provider != "" || eff.Scoring != scoring.DefaultConfig() {
		t.Fatalf("nil config not handled: %+v", eff)
	}
}

func TestApplyRubricOverridesOnlySpecifiedFields(t *testing.T) {
	base := scoring.DefaultConfig()
	agg := 15
	r := &models.RubricConfig{
		Weights:         map[string]float64{"severity": 0.9}, // only severity changes
		Tiers:           &models.TierThresholds{Must: 95, Should: 70, May: 40},
		AggregatorScore: &agg,
		// EvergreenCap / PromoCap left nil -> keep base
	}
	got := applyRubric(base, r)

	if got.Weights.Severity != 0.9 {
		t.Errorf("Severity = %v, want 0.9", got.Weights.Severity)
	}
	if got.Weights.Specificity != base.Weights.Specificity {
		t.Errorf("Specificity = %v, want unchanged base %v", got.Weights.Specificity, base.Weights.Specificity)
	}
	if got.TierMust != 95 || got.TierShould != 70 || got.TierMay != 40 {
		t.Errorf("tiers = %d/%d/%d, want 95/70/40", got.TierMust, got.TierShould, got.TierMay)
	}
	if got.AggregatorScore != 15 {
		t.Errorf("AggregatorScore = %d, want 15", got.AggregatorScore)
	}
	if got.EvergreenCap != base.EvergreenCap || got.PromoCap != base.PromoCap {
		t.Errorf("caps changed unexpectedly: %d/%d, want base %d/%d",
			got.EvergreenCap, got.PromoCap, base.EvergreenCap, base.PromoCap)
	}
}

func TestWithRequestOverrides(t *testing.T) {
	base := EffectiveEditorial{VibeScore: false, Glossary: true, StandardSynthesis: false}

	t.Run("nil request is identity", func(t *testing.T) {
		got := base.withRequestOverrides(nil)
		if got.VibeScore != base.VibeScore || got.Glossary != base.Glossary ||
			got.StandardSynthesis != base.StandardSynthesis {
			t.Fatalf("withRequestOverrides(nil) = %+v, want unchanged", got)
		}
	})

	t.Run("non-nil pointers win", func(t *testing.T) {
		req := &protos.AnalyzeArticleWithProviderModelRequest{
			VibeScore:         boolPtr(true),
			StandardSynthesis: boolPtr(true),
			// Glossary nil -> keep base's true
		}
		got := base.withRequestOverrides(req)
		if !got.VibeScore {
			t.Error("VibeScore not overridden to true")
		}
		if !got.StandardSynthesis {
			t.Error("StandardSynthesis not overridden to true")
		}
		if !got.Glossary {
			t.Error("Glossary changed despite nil override")
		}
	})
}
