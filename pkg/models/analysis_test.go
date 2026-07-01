package models

import (
	"testing"
)

func TestArticleAnalysisBeforeCreateGeneratesIDWhenEmpty(t *testing.T) {
	a := &ArticleAnalysis{}
	if err := a.BeforeCreate(nil); err != nil {
		t.Fatalf("BeforeCreate() error = %v", err)
	}
	if a.Id == "" {
		t.Fatal("Id still empty after BeforeCreate, want a generated UUID")
	}
}

func TestArticleAnalysisBeforeCreatePreservesExplicitID(t *testing.T) {
	a := &ArticleAnalysis{Id: "fixed-id"}
	if err := a.BeforeCreate(nil); err != nil {
		t.Fatalf("BeforeCreate() error = %v", err)
	}
	if a.Id != "fixed-id" {
		t.Fatalf("Id = %q, want the explicit fixed-id preserved", a.Id)
	}
}

func TestArticleAnalysisBeforeCreateSerializesSlices(t *testing.T) {
	a := &ArticleAnalysis{
		KeyPoints:     []string{"one", "two"},
		Insights:      []string{"deep"},
		GlossaryTerms: []GlossaryTerm{{Term: "RCE", Definition: "remote code exec"}},
	}
	if err := a.BeforeCreate(nil); err != nil {
		t.Fatalf("BeforeCreate() error = %v", err)
	}
	if a.KeyPointsJson != `["one","two"]` {
		t.Errorf("KeyPointsJson = %q, want serialized array", a.KeyPointsJson)
	}
	if a.InsightsJson != `["deep"]` {
		t.Errorf("InsightsJson = %q", a.InsightsJson)
	}
	if a.GlossaryTermsJson == "" {
		t.Error("GlossaryTermsJson empty, want serialized glossary terms")
	}
}

func TestArticleAnalysisBeforeCreateLeavesEmptySlicesUnserialized(t *testing.T) {
	a := &ArticleAnalysis{Id: "x"} // no slices
	if err := a.BeforeCreate(nil); err != nil {
		t.Fatalf("BeforeCreate() error = %v", err)
	}
	if a.KeyPointsJson != "" || a.InsightsJson != "" || a.GlossaryTermsJson != "" {
		t.Fatalf("empty slices produced JSON: kp=%q ins=%q gl=%q",
			a.KeyPointsJson, a.InsightsJson, a.GlossaryTermsJson)
	}
}

func TestArticleAnalysisAfterFindDeserializes(t *testing.T) {
	a := &ArticleAnalysis{
		KeyPointsJson:     `["a","b"]`,
		InsightsJson:      `["insight"]`,
		GlossaryTermsJson: `[{"term":"RCE","definition":"remote code exec"}]`,
	}
	if err := a.AfterFind(nil); err != nil {
		t.Fatalf("AfterFind() error = %v", err)
	}
	if len(a.KeyPoints) != 2 || a.KeyPoints[0] != "a" || a.KeyPoints[1] != "b" {
		t.Errorf("KeyPoints = %v, want [a b]", a.KeyPoints)
	}
	if len(a.Insights) != 1 || a.Insights[0] != "insight" {
		t.Errorf("Insights = %v, want [insight]", a.Insights)
	}
	if len(a.GlossaryTerms) != 1 || a.GlossaryTerms[0].Term != "RCE" {
		t.Errorf("GlossaryTerms = %v, want one RCE term", a.GlossaryTerms)
	}
}

func TestArticleAnalysisJSONHooksRoundTrip(t *testing.T) {
	orig := &ArticleAnalysis{
		Id:                "keep",
		KeyPoints:         []string{"x", "y", "z"},
		ReferencedReports: []ReferencedReport{{Title: "CVE-2026-1"}},
	}
	if err := orig.BeforeCreate(nil); err != nil {
		t.Fatalf("BeforeCreate() error = %v", err)
	}
	// Simulate a DB read: only the *Json columns persist.
	reloaded := &ArticleAnalysis{
		KeyPointsJson:         orig.KeyPointsJson,
		ReferencedReportsJson: orig.ReferencedReportsJson,
	}
	if err := reloaded.AfterFind(nil); err != nil {
		t.Fatalf("AfterFind() error = %v", err)
	}
	if len(reloaded.KeyPoints) != 3 || reloaded.KeyPoints[2] != "z" {
		t.Errorf("KeyPoints round-trip = %v, want [x y z]", reloaded.KeyPoints)
	}
	if len(reloaded.ReferencedReports) != 1 || reloaded.ReferencedReports[0].Title != "CVE-2026-1" {
		t.Errorf("ReferencedReports round-trip lost: %v", reloaded.ReferencedReports)
	}
}
