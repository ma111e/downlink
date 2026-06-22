package services

import (
	"github.com/ma111e/downlink/cmd/server/internal/config"
	"github.com/ma111e/downlink/cmd/server/internal/store"
	"github.com/ma111e/downlink/pkg/models"
	"github.com/ma111e/downlink/pkg/protos"
	"github.com/ma111e/downlink/pkg/scoring"

	log "github.com/sirupsen/logrus"
)

// EffectiveEditorial is a fully-resolved editorial config for one analysis run:
// the profile's choices layered over the live global AnalysisConfig, with the
// rubric expressed as a concrete scoring.Config. It is the single object the
// analysis pipeline reads, so the pipeline never touches config.Config or a
// profile directly.
type EffectiveEditorial struct {
	ProfileId              string
	Provider               string
	Model                  string
	Persona                string
	WritingStyle           string
	Audience               string
	VibeScore              bool
	Glossary               bool
	StandardSynthesis      bool
	ComprehensiveSynthesis bool
	ExecutiveSummary       bool
	Categories             []models.CategoryDef // empty = default allowedCategories
	Scoring                scoring.Config
	Prompts                models.PromptOverrides
}

// ResolveEditorial layers a profile's editorial over the LIVE global
// AnalysisConfig. Any empty/nil field in ed inherits the global value, so the
// "default" profile (which stores an empty editorial) always behaves exactly
// like today's single-tenant config — and keeps tracking config.json changes.
func ResolveEditorial(profileId string, ed *models.ProfileEditorial) EffectiveEditorial {
	var base models.AnalysisConfig
	if config.Config != nil {
		base = config.Config.Analysis
	}

	eff := EffectiveEditorial{
		ProfileId:              profileId,
		Provider:               base.Provider,
		Persona:                base.Persona,
		WritingStyle:           base.WritingStyle,
		VibeScore:              base.VibeScore,
		Glossary:               base.Glossary,
		StandardSynthesis:      base.StandardSynthesis,
		ComprehensiveSynthesis: base.ComprehensiveSynthesis,
		ExecutiveSummary:       base.ExecutiveSummary,
		Scoring:                scoring.DefaultConfig(),
	}

	if ed == nil {
		return eff
	}

	if ed.Provider != "" {
		eff.Provider = ed.Provider
	}
	if ed.Model != "" {
		eff.Model = ed.Model
	}
	if ed.Persona != "" {
		eff.Persona = ed.Persona
	}
	if ed.WritingStyle != "" {
		eff.WritingStyle = ed.WritingStyle
	}
	if ed.Audience != "" {
		eff.Audience = ed.Audience
	}
	if ed.Glossary != nil {
		eff.Glossary = *ed.Glossary
	}
	if ed.VibeScore != nil {
		eff.VibeScore = *ed.VibeScore
	}
	if ed.StandardSynthesis != nil {
		eff.StandardSynthesis = *ed.StandardSynthesis
	}
	if ed.ComprehensiveSynthesis != nil {
		eff.ComprehensiveSynthesis = *ed.ComprehensiveSynthesis
	}
	if ed.ExecutiveSummary != nil {
		eff.ExecutiveSummary = *ed.ExecutiveSummary
	}
	if len(ed.Categories) > 0 {
		eff.Categories = ed.Categories
	}
	if ed.Rubric != nil {
		eff.Scoring = applyRubric(eff.Scoring, ed.Rubric)
	}
	if ed.Prompts != nil {
		eff.Prompts = *ed.Prompts
	}
	return eff
}

// applyRubric overlays a profile's rubric config onto a base scoring.Config.
// Unspecified weights/thresholds keep the base (default) values.
func applyRubric(c scoring.Config, r *models.RubricConfig) scoring.Config {
	if r.Weights != nil {
		w := c.Weights
		if v, ok := r.Weights["specificity"]; ok {
			w.Specificity = v
		}
		if v, ok := r.Weights["severity"]; ok {
			w.Severity = v
		}
		if v, ok := r.Weights["breadth"]; ok {
			w.Breadth = v
		}
		if v, ok := r.Weights["novelty"]; ok {
			w.Novelty = v
		}
		if v, ok := r.Weights["actionability"]; ok {
			w.Actionability = v
		}
		if v, ok := r.Weights["credibility"]; ok {
			w.Credibility = v
		}
		c.Weights = w
	}
	if r.Tiers != nil {
		c.TierMust = r.Tiers.Must
		c.TierShould = r.Tiers.Should
		c.TierMay = r.Tiers.May
	}
	if r.AggregatorScore != nil {
		c.AggregatorScore = *r.AggregatorScore
	}
	if r.EvergreenCap != nil {
		c.EvergreenCap = *r.EvergreenCap
	}
	if r.PromoCap != nil {
		c.PromoCap = *r.PromoCap
	}
	return c
}

// withRequestOverrides applies per-run override pointers from an analysis
// request (e.g. the --beginner / --vibe-score flags). A non-nil override wins
// over both the profile and the global default.
func (e EffectiveEditorial) withRequestOverrides(req *protos.AnalyzeArticleWithProviderModelRequest) EffectiveEditorial {
	if req == nil {
		return e
	}
	if req.VibeScore != nil {
		e.VibeScore = *req.VibeScore
	}
	if req.Glossary != nil {
		e.Glossary = *req.Glossary
	}
	if req.StandardSynthesis != nil {
		e.StandardSynthesis = *req.StandardSynthesis
	}
	if req.ComprehensiveSynthesis != nil {
		e.ComprehensiveSynthesis = *req.ComprehensiveSynthesis
	}
	return e
}

// editorialForRequest resolves the editorial config for an analysis request: it
// loads the request's profile (defaulting to "default" when unset) and layers
// the profile's editorial over the live global config, then applies any per-run
// request overrides. The default profile stores an empty editorial, so it
// resolves to the live global config — i.e. today's setup, now expressed as a
// profile rather than a special case.
func (s *LLMsServer) editorialForRequest(req *protos.AnalyzeArticleWithProviderModelRequest) EffectiveEditorial {
	slug := defaultProfileId
	if req != nil && req.ProfileSlug != "" {
		slug = req.ProfileSlug
	}

	var ed *models.ProfileEditorial
	if profile, err := store.Db.GetProfile(slug); err != nil {
		log.WithError(err).WithField("profile", slug).Warn("failed to load profile for analysis; falling back to global editorial config")
	} else {
		ed = profile.Editorial
	}

	return ResolveEditorial(slug, ed).withRequestOverrides(req)
}

// resolveDigestEditorial resolves the editorial config for a whole digest run:
// the profile's editorial layered over the live global config, with the
// digest-level per-run override flags applied on top. It governs article
// selection scope, dedupe/summary prompts, and the executive-summary/glossary
// gating, and is passed down to per-article analysis.
func resolveDigestEditorial(req *protos.GenerateDigestRequest, profile models.Profile) EffectiveEditorial {
	ed := ResolveEditorial(profile.Id, profile.Editorial)
	if req == nil {
		return ed
	}
	if req.VibeScore != nil {
		ed.VibeScore = *req.VibeScore
	}
	if req.Glossary != nil {
		ed.Glossary = *req.Glossary
	}
	if req.StandardSynthesis != nil {
		ed.StandardSynthesis = *req.StandardSynthesis
	}
	if req.ComprehensiveSynthesis != nil {
		ed.ComprehensiveSynthesis = *req.ComprehensiveSynthesis
	}
	if req.ExecutiveSummary != nil {
		ed.ExecutiveSummary = *req.ExecutiveSummary
	}
	return ed
}

// defaultProfileId is the slug of the always-present profile seeded by the store.
const defaultProfileId = "default"
