package store

import (
	"testing"
	"time"

	"github.com/ma111e/downlink/pkg/models"
)

func TestStoreAndGetDigestAnalysis(t *testing.T) {
	s := newTestStore(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	if err := s.SaveArticleAnalysis(&models.ArticleAnalysis{
		Id: "an1", ArticleId: "art1", ProfileId: "p1", CreatedAt: base,
	}); err != nil {
		t.Fatalf("SaveArticleAnalysis: %v", err)
	}

	entry := models.DigestAnalysis{
		DigestId:            "d1",
		AnalysisId:          "an1",
		ArticleId:           "art1",
		DuplicateGroup:      "grp-a",
		IsMostComprehensive: true,
	}
	if err := s.StoreDigestAnalysis(entry); err != nil {
		t.Fatalf("StoreDigestAnalysis: %v", err)
	}

	got, err := s.GetDigestAnalyses("d1")
	if err != nil {
		t.Fatalf("GetDigestAnalyses: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].AnalysisId != "an1" || got[0].ArticleId != "art1" {
		t.Errorf("got %+v, want an1/art1", got[0])
	}
	if !got[0].IsMostComprehensive || got[0].DuplicateGroup != "grp-a" {
		t.Errorf("fields lost: IsMostComprehensive=%v DuplicateGroup=%q", got[0].IsMostComprehensive, got[0].DuplicateGroup)
	}
}

func TestGetDigestAnalysesPreloadsAnalysis(t *testing.T) {
	s := newTestStore(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	if err := s.SaveArticleAnalysis(&models.ArticleAnalysis{
		Id: "an2", ArticleId: "art2", ProfileId: "p1", CreatedAt: base,
		Tldr: "preload check",
	}); err != nil {
		t.Fatalf("SaveArticleAnalysis: %v", err)
	}
	if err := s.StoreDigestAnalysis(models.DigestAnalysis{DigestId: "d2", AnalysisId: "an2", ArticleId: "art2"}); err != nil {
		t.Fatalf("StoreDigestAnalysis: %v", err)
	}

	got, err := s.GetDigestAnalyses("d2")
	if err != nil {
		t.Fatalf("GetDigestAnalyses: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Analysis == nil {
		t.Fatal("Analysis not preloaded")
	}
	if got[0].Analysis.Tldr != "preload check" {
		t.Errorf("Analysis.Tldr = %q, want \"preload check\"", got[0].Analysis.Tldr)
	}
}

func TestStoreDigestAnalysesBatch(t *testing.T) {
	s := newTestStore(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for _, id := range []string{"an3", "an4"} {
		if err := s.SaveArticleAnalysis(&models.ArticleAnalysis{
			Id: id, ArticleId: "art" + id, ProfileId: "p1", CreatedAt: base,
		}); err != nil {
			t.Fatalf("SaveArticleAnalysis(%s): %v", id, err)
		}
	}

	batch := []models.DigestAnalysis{
		{DigestId: "d3", AnalysisId: "an3", ArticleId: "artan3"},
		{DigestId: "d3", AnalysisId: "an4", ArticleId: "artan4"},
	}
	if err := s.StoreDigestAnalysesBatch(batch); err != nil {
		t.Fatalf("StoreDigestAnalysesBatch: %v", err)
	}

	got, err := s.GetDigestAnalyses("d3")
	if err != nil {
		t.Fatalf("GetDigestAnalyses: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len = %d, want 2", len(got))
	}
}

func TestStoreDigestAnalysesBatchEmptyIsNoop(t *testing.T) {
	s := newTestStore(t)
	if err := s.StoreDigestAnalysesBatch(nil); err != nil {
		t.Errorf("StoreDigestAnalysesBatch(nil) error = %v, want nil", err)
	}
	if err := s.StoreDigestAnalysesBatch([]models.DigestAnalysis{}); err != nil {
		t.Errorf("StoreDigestAnalysesBatch([]) error = %v, want nil", err)
	}
}

func TestGetDigestAnalysesUnknownDigestReturnsEmpty(t *testing.T) {
	s := newTestStore(t)
	got, err := s.GetDigestAnalyses("ghost")
	if err != nil {
		t.Fatalf("GetDigestAnalyses(ghost) error = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}
