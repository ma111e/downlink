package store

import (
	"testing"
	"time"

	"github.com/ma111e/downlink/pkg/models"
)

func saveAnalysis(t *testing.T, s *GormStore, id, articleID, profileID string, created time.Time) {
	t.Helper()
	a := &models.ArticleAnalysis{Id: id, ArticleId: articleID, ProfileId: profileID, CreatedAt: created}
	if err := s.SaveArticleAnalysis(a); err != nil {
		t.Fatalf("SaveArticleAnalysis(%s) error = %v", id, err)
	}
}

func TestGetArticleAnalysisReturnsMostRecent(t *testing.T) {
	s := newTestStore(t)
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	saveAnalysis(t, s, "an-old", "a1", "", base)
	saveAnalysis(t, s, "an-new", "a1", "", base.Add(time.Hour))

	got, err := s.GetArticleAnalysis("a1", "")
	if err != nil {
		t.Fatalf("GetArticleAnalysis() error = %v", err)
	}
	if got.Id != "an-new" {
		t.Fatalf("GetArticleAnalysis() = %q, want most recent an-new", got.Id)
	}
}

func TestGetArticleAnalysisFiltersByProfile(t *testing.T) {
	s := newTestStore(t)
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	// The newest overall belongs to profile "p2"; asking for "p1" must skip it.
	saveAnalysis(t, s, "p1-analysis", "a1", "p1", base)
	saveAnalysis(t, s, "p2-analysis", "a1", "p2", base.Add(time.Hour))

	got, err := s.GetArticleAnalysis("a1", "p1")
	if err != nil {
		t.Fatalf("GetArticleAnalysis() error = %v", err)
	}
	if got.Id != "p1-analysis" || got.ProfileId != "p1" {
		t.Fatalf("got %+v, want p1-analysis for profile p1", got)
	}
}

func TestGetArticleAnalysisNotFound(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.GetArticleAnalysis("ghost", ""); err == nil {
		t.Fatal("GetArticleAnalysis(ghost) error = nil, want not-found")
	}
}

func TestGetArticleAnalysesBatchKeepsMostRecentPerArticle(t *testing.T) {
	s := newTestStore(t)
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	saveAnalysis(t, s, "a1-old", "a1", "", base)
	saveAnalysis(t, s, "a1-new", "a1", "", base.Add(time.Hour))
	saveAnalysis(t, s, "a2-only", "a2", "", base)

	m, err := s.GetArticleAnalysesBatch([]string{"a1", "a2", "a3"}, "")
	if err != nil {
		t.Fatalf("GetArticleAnalysesBatch() error = %v", err)
	}
	if len(m) != 2 {
		t.Fatalf("map has %d entries, want 2 (a3 has none)", len(m))
	}
	if m["a1"].Id != "a1-new" {
		t.Errorf("a1 = %q, want most recent a1-new", m["a1"].Id)
	}
	if m["a2"].Id != "a2-only" {
		t.Errorf("a2 = %q, want a2-only", m["a2"].Id)
	}
	if _, ok := m["a3"]; ok {
		t.Error("a3 present in map, want absent (no analysis)")
	}
}

func TestGetArticleAnalysesBatchEmptyInput(t *testing.T) {
	s := newTestStore(t)
	m, err := s.GetArticleAnalysesBatch(nil, "")
	if err != nil {
		t.Fatalf("GetArticleAnalysesBatch(nil) error = %v", err)
	}
	if len(m) != 0 {
		t.Fatalf("map = %v, want empty for empty input", m)
	}
}

func TestGetAllArticleAnalysesNewestFirst(t *testing.T) {
	s := newTestStore(t)
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	saveAnalysis(t, s, "old", "a1", "", base)
	saveAnalysis(t, s, "new", "a1", "", base.Add(time.Hour))
	saveAnalysis(t, s, "other", "a2", "", base)

	all, err := s.GetAllArticleAnalyses("a1", "")
	if err != nil {
		t.Fatalf("GetAllArticleAnalyses() error = %v", err)
	}
	if len(all) != 2 || all[0].Id != "new" || all[1].Id != "old" {
		t.Fatalf("got %d analyses in order %q,%q, want [new old]", len(all),
			firstID(all, 0), firstID(all, 1))
	}
}

func firstID(a []models.ArticleAnalysis, i int) string {
	if i < len(a) {
		return a[i].Id
	}
	return ""
}

func TestUpdateArticleAnalysisGlossaryTerms(t *testing.T) {
	s := newTestStore(t)
	saveAnalysis(t, s, "an1", "a1", "", time.Now())

	if err := s.UpdateArticleAnalysisGlossaryTerms("an1", `[{"term":"x"}]`); err != nil {
		t.Fatalf("UpdateArticleAnalysisGlossaryTerms() error = %v", err)
	}
	var got models.ArticleAnalysis
	if err := s.db.First(&got, "id = ?", "an1").Error; err != nil {
		t.Fatalf("reload error = %v", err)
	}
	if got.GlossaryTermsJson != `[{"term":"x"}]` {
		t.Fatalf("glossary_terms = %q, want the updated JSON", got.GlossaryTermsJson)
	}

	if err := s.UpdateArticleAnalysisGlossaryTerms("", "x"); err == nil {
		t.Error("UpdateArticleAnalysisGlossaryTerms(empty id) = nil, want error")
	}
}
