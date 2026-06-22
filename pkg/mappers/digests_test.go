package mappers

import (
	"testing"

	"github.com/ma111e/downlink/pkg/models"
)

// TestDigestRoundTripsGlossaryAndLearning guards the republish render parity:
// the glossary panel and "Learning" button are driven by DigestGlossary and the
// per-article PlainWords / GlossaryTerms fields. These
// must survive the proto round-trip the CLI publish paths rely on, otherwise
// republished pages silently drop the glossary and learning UI.
func TestDigestRoundTripsGlossaryAndLearning(t *testing.T) {
	input := &models.Digest{
		Id:    "digest-1",
		Title: "Weekly Threat Intel",
		DigestGlossary: []models.DigestGlossary{
			{
				DigestId: "digest-1",
				EntryId:  "entry-1",
				Entry: &models.GlossaryEntry{
					Id:            "entry-1",
					NormalizedKey: "cobalt strike",
					Term:          "Cobalt Strike",
					Kind:          models.GlossaryKindEntity,
					Category:      "tool",
					Difficulty:    "advanced",
					Definition:    "A commercial adversary simulation tool.",
					TagId:         "cobalt-strike",
				},
			},
		},
		DigestAnalyses: []models.DigestAnalysis{
			{
				DigestId:   "digest-1",
				AnalysisId: "analysis-1",
				ArticleId:  "article-1",
				Analysis: &models.ArticleAnalysis{
					Id:         "analysis-1",
					ArticleId:  "article-1",
					PlainWords: "It signals active exploitation, affecting ordinary users.",
					GlossaryTerms: []models.GlossaryTerm{
						{Term: "Cobalt Strike", Type: "tool", Definition: "Adversary sim.", Context: "Used to stage the intrusion."},
					},
				},
			},
		},
	}

	out := DigestToModel(DigestToProto(input))

	if len(out.DigestGlossary) != 1 {
		t.Fatalf("DigestGlossary not preserved: got %d entries", len(out.DigestGlossary))
	}
	dg := out.DigestGlossary[0]
	if dg.Entry == nil {
		t.Fatal("DigestGlossary.Entry was dropped")
	}
	if dg.Entry.Term != "Cobalt Strike" || dg.Entry.NormalizedKey != "cobalt strike" {
		t.Errorf("glossary entry fields lost: %+v", dg.Entry)
	}
	if dg.Entry.EffectiveDefinition() != "A commercial adversary simulation tool." {
		t.Errorf("glossary definition lost: %q", dg.Entry.EffectiveDefinition())
	}
	if dg.Entry.Difficulty != "advanced" {
		t.Errorf("glossary difficulty lost: %q", dg.Entry.Difficulty)
	}

	if len(out.DigestAnalyses) != 1 || out.DigestAnalyses[0].Analysis == nil {
		t.Fatal("DigestAnalyses/Analysis dropped")
	}
	a := out.DigestAnalyses[0].Analysis
	if a.PlainWords != "It signals active exploitation, affecting ordinary users." {
		t.Errorf("PlainWords lost: %q", a.PlainWords)
	}
	if len(a.GlossaryTerms) != 1 {
		t.Fatalf("GlossaryTerms not preserved: got %d", len(a.GlossaryTerms))
	}
	gt := a.GlossaryTerms[0]
	if gt.Term != "Cobalt Strike" || gt.Context != "Used to stage the intrusion." {
		t.Errorf("glossary term fields lost: %+v", gt)
	}
}

// TestAnalysisConfigRoundTrip guards against the `dlk model` data-loss bug: the
// proto round-trip the CLI uses to load and save the analysis config must
// preserve every setting, not just Provider/Persona. Otherwise switching the
// provider silently resets vibe scoring, glossary, synthesis flags, etc.
func TestAnalysisConfigRoundTrip(t *testing.T) {
	maxWorkers := 7
	input := &models.AnalysisConfig{
		Provider:               "openai-main",
		Persona:                "terse analyst",
		WritingStyle:           "plain and direct",
		WorkerPool:             &models.WorkerPoolConfig{MaxWorkers: &maxWorkers},
		AutoAnalyze:            true,
		VibeScore:              true,
		Glossary:               true,
		StandardSynthesis:      true,
		ComprehensiveSynthesis: true,
		ExecutiveSummary:       true,
	}

	out := AnalysisConfigToModel(AnalysisConfigToProto(input))
	if out == nil {
		t.Fatal("AnalysisConfig round-trip returned nil")
	}

	if out.Provider != input.Provider || out.Persona != input.Persona ||
		out.WritingStyle != input.WritingStyle || out.AutoAnalyze != input.AutoAnalyze ||
		out.VibeScore != input.VibeScore || out.Glossary != input.Glossary ||
		out.StandardSynthesis != input.StandardSynthesis ||
		out.ComprehensiveSynthesis != input.ComprehensiveSynthesis ||
		out.ExecutiveSummary != input.ExecutiveSummary {
		t.Errorf("scalar analysis fields lost: %+v", out)
	}
	if out.WorkerPool == nil || out.WorkerPool.MaxWorkers == nil {
		t.Fatalf("worker pool dropped: %+v", out.WorkerPool)
	}
	if *out.WorkerPool.MaxWorkers != maxWorkers {
		t.Errorf("MaxWorkers lost: got %d, want %d", *out.WorkerPool.MaxWorkers, maxWorkers)
	}
}
