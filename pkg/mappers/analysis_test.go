package mappers

import (
	"testing"
	"time"

	"github.com/ma111e/downlink/pkg/models"
	"github.com/ma111e/downlink/pkg/scoring"
)

func TestArticleAnalysisRoundTripPreservesFields(t *testing.T) {
	created := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	in := &models.ArticleAnalysis{
		Id:                     "an1",
		ArticleId:              "art1",
		ProviderType:           "claude",
		ModelName:              "claude-opus",
		ImportanceScore:        87,
		Tldr:                   "short summary",
		Justification:          "why it matters",
		BriefOverview:          "brief text",
		StandardSynthesis:      "standard text",
		ComprehensiveSynthesis: "comprehensive text",
		ThinkingProcess:        "reasoning",
		RawResponse:            "raw json",
		PlainWords:             "plain explanation",
		KeyPoints:              []string{"kp1", "kp2"},
		Insights:               []string{"ins1"},
		KeyPointsJson:          `["kp1","kp2"]`,
		InsightsJson:           `["ins1"]`,
		ReferencedReports: []models.ReferencedReport{
			{Title: "Report A", URL: "https://x/r", Publisher: "Pub", Context: "ctx"},
		},
		ReferencedReportsJson: `[{"title":"Report A","url":"https://x/r","publisher":"Pub","context":"ctx"}]`,
		GlossaryTerms: []models.GlossaryTerm{
			{Term: "CVE", Type: "acronym", Definition: "Common Vulnerability", Context: "used here"},
		},
		ScoreDimensions: &scoring.Dimensions{
			Specificity: 3, Severity: 4, Breadth: 2, Novelty: 1, Actionability: 3, Credibility: 4,
			IsAggregator: false, IsPromotional: true,
		},
		CreatedAt: created,
	}

	out := ArticleAnalysisToModel(ArticleAnalysisToProto(in))

	if out.Id != "an1" || out.ArticleId != "art1" {
		t.Errorf("id/articleId lost: %q %q", out.Id, out.ArticleId)
	}
	if out.ProviderType != "claude" || out.ModelName != "claude-opus" {
		t.Errorf("provider/model lost: %q %q", out.ProviderType, out.ModelName)
	}
	if out.ImportanceScore != 87 {
		t.Errorf("ImportanceScore = %d, want 87", out.ImportanceScore)
	}
	if out.Tldr != "short summary" || out.Justification != "why it matters" {
		t.Errorf("text fields lost: tldr=%q just=%q", out.Tldr, out.Justification)
	}
	if out.BriefOverview != "brief text" || out.StandardSynthesis != "standard text" ||
		out.ComprehensiveSynthesis != "comprehensive text" || out.ThinkingProcess != "reasoning" ||
		out.RawResponse != "raw json" || out.PlainWords != "plain explanation" {
		t.Error("blob text fields lost")
	}
	if len(out.KeyPoints) != 2 || out.KeyPoints[0] != "kp1" {
		t.Errorf("KeyPoints = %v", out.KeyPoints)
	}
	if len(out.Insights) != 1 || out.Insights[0] != "ins1" {
		t.Errorf("Insights = %v", out.Insights)
	}
	if len(out.ReferencedReports) != 1 || out.ReferencedReports[0].Title != "Report A" || out.ReferencedReports[0].URL != "https://x/r" {
		t.Errorf("ReferencedReports = %+v", out.ReferencedReports)
	}
	if len(out.GlossaryTerms) != 1 || out.GlossaryTerms[0].Term != "CVE" || out.GlossaryTerms[0].Definition != "Common Vulnerability" {
		t.Errorf("GlossaryTerms = %+v", out.GlossaryTerms)
	}
	if out.ScoreDimensions == nil || out.ScoreDimensions.Specificity != 3 || out.ScoreDimensions.Severity != 4 {
		t.Errorf("ScoreDimensions = %+v", out.ScoreDimensions)
	}
	if out.ScoreDimensions.IsPromotional != true || out.ScoreDimensions.IsAggregator != false {
		t.Errorf("ScoreDimensions bool fields lost")
	}
	if !out.CreatedAt.Equal(created) {
		t.Errorf("CreatedAt = %v, want %v", out.CreatedAt, created)
	}
}

func TestArticleAnalysisNilIsNil(t *testing.T) {
	if ArticleAnalysisToProto(nil) != nil {
		t.Fatal("ArticleAnalysisToProto(nil) != nil")
	}
	if ArticleAnalysisToModel(nil) != nil {
		t.Fatal("ArticleAnalysisToModel(nil) != nil")
	}
}

func TestAllArticleAnalysesToProtoAndBack(t *testing.T) {
	in := []models.ArticleAnalysis{
		{Id: "a1", ArticleId: "art1", ImportanceScore: 50},
		{Id: "a2", ArticleId: "art2", ImportanceScore: 75},
	}
	out := AllArticleAnalysesToModels(AllArticleAnalysesToProto(in))
	if len(out) != 2 || out[0].Id != "a1" || out[1].Id != "a2" {
		t.Errorf("slice round-trip lost data: %+v", out)
	}
	if out[0].ImportanceScore != 50 || out[1].ImportanceScore != 75 {
		t.Errorf("scores lost: %d %d", out[0].ImportanceScore, out[1].ImportanceScore)
	}
}

func TestAllArticleAnalysesToModelsSkipsNil(t *testing.T) {
	out := AllArticleAnalysesToModels(nil)
	if out != nil {
		t.Errorf("AllArticleAnalysesToModels(nil) = %v, want nil", out)
	}
}

func TestScoreDimensionsRoundTrip(t *testing.T) {
	in := &scoring.Dimensions{
		Specificity: 1, Severity: 2, Breadth: 3, Novelty: 4,
		Actionability: 2, Credibility: 3,
		IsAggregator: true, IsPromotional: false,
	}
	out := ScoreDimensionsToModel(ScoreDimensionsToProto(in))
	if out == nil {
		t.Fatal("ScoreDimensionsToModel returned nil")
	}
	if out.Specificity != 1 || out.Severity != 2 || out.Breadth != 3 || out.Novelty != 4 ||
		out.Actionability != 2 || out.Credibility != 3 {
		t.Errorf("int fields lost: %+v", out)
	}
	if !out.IsAggregator || out.IsPromotional {
		t.Errorf("bool fields lost: IsAggregator=%v IsPromotional=%v", out.IsAggregator, out.IsPromotional)
	}
}

func TestScoreDimensionsNilIsNil(t *testing.T) {
	if ScoreDimensionsToProto(nil) != nil {
		t.Fatal("ScoreDimensionsToProto(nil) != nil")
	}
	if ScoreDimensionsToModel(nil) != nil {
		t.Fatal("ScoreDimensionsToModel(nil) != nil")
	}
}

func TestReferencedReportRoundTrip(t *testing.T) {
	in := models.ReferencedReport{
		Title: "T", URL: "https://u", Publisher: "P", Context: "C",
	}
	proto := ReferencedReportToProto(in)
	out := ReferencedReportToModel(proto)
	if out.Title != "T" || out.URL != "https://u" || out.Publisher != "P" || out.Context != "C" {
		t.Errorf("round-trip lost: %+v", out)
	}
}

func TestReferencedReportToModelNilReturnsZero(t *testing.T) {
	out := ReferencedReportToModel(nil)
	if out != (models.ReferencedReport{}) {
		t.Errorf("nil proto gave non-zero result: %+v", out)
	}
}

func TestAllReferencedReportsRoundTrip(t *testing.T) {
	in := []models.ReferencedReport{
		{Title: "R1", URL: "https://a"},
		{Title: "R2", URL: "https://b"},
	}
	out := AllReferencedReportsToModels(AllReferencedReportsToProto(in))
	if len(out) != 2 || out[0].Title != "R1" || out[1].Title != "R2" {
		t.Errorf("slice round-trip lost data: %+v", out)
	}
}

func TestAllReferencedReportsToModelsSkipsNil(t *testing.T) {
	protos := AllReferencedReportsToProto([]models.ReferencedReport{{Title: "R"}})
	protos = append(protos, nil)
	out := AllReferencedReportsToModels(protos)
	if len(out) != 1 {
		t.Errorf("len = %d, want 1 (nil entry should be skipped)", len(out))
	}
}

func TestGlossaryTermRoundTrip(t *testing.T) {
	in := models.GlossaryTerm{Term: "CVE", Type: "acronym", Definition: "vuln id", Context: "used here"}
	out := GlossaryTermToModel(GlossaryTermToProto(in))
	if out.Term != "CVE" || out.Type != "acronym" || out.Definition != "vuln id" || out.Context != "used here" {
		t.Errorf("round-trip lost: %+v", out)
	}
}

func TestGlossaryTermToModelNilReturnsZero(t *testing.T) {
	out := GlossaryTermToModel(nil)
	if out.Term != "" || out.Type != "" || out.Definition != "" || out.Context != "" {
		t.Errorf("nil proto gave non-zero result: %+v", out)
	}
}

func TestAllGlossaryTermsRoundTrip(t *testing.T) {
	in := []models.GlossaryTerm{
		{Term: "APT", Type: "acronym", Definition: "advanced persistent threat"},
		{Term: "IOC", Type: "acronym", Definition: "indicator of compromise"},
	}
	out := AllGlossaryTermsToModels(AllGlossaryTermsToProto(in))
	if len(out) != 2 || out[0].Term != "APT" || out[1].Term != "IOC" {
		t.Errorf("slice round-trip lost data: %+v", out)
	}
}
