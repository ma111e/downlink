package services

import (
	"context"
	"testing"
	"time"

	"github.com/ma111e/downlink/cmd/server/internal/store"
	"github.com/ma111e/downlink/pkg/models"
	"github.com/ma111e/downlink/pkg/protos"
)

func TestGetAllArticleAnalysesEmpty(t *testing.T) {
	withTempStore(t)
	srv := NewAnalysisServer()
	resp, err := srv.GetAllArticleAnalyses(context.Background(), &protos.GetAllArticleAnalysesRequest{ArticleId: "art1"})
	if err != nil {
		t.Fatalf("GetAllArticleAnalyses error = %v", err)
	}
	if len(resp.Analyses) != 0 {
		t.Errorf("len = %d, want 0", len(resp.Analyses))
	}
}

func TestGetAllArticleAnalysesReturnsAll(t *testing.T) {
	withTempStore(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, a := range []*models.ArticleAnalysis{
		{Id: "a1", ArticleId: "art1", ProfileId: "p1", CreatedAt: base},
		{Id: "a2", ArticleId: "art1", ProfileId: "p2", CreatedAt: base.Add(time.Hour)},
	} {
		if err := store.Db.SaveArticleAnalysis(a); err != nil {
			t.Fatalf("SaveArticleAnalysis(%s) error = %v", a.Id, err)
		}
	}

	srv := NewAnalysisServer()
	resp, err := srv.GetAllArticleAnalyses(context.Background(), &protos.GetAllArticleAnalysesRequest{ArticleId: "art1"})
	if err != nil {
		t.Fatalf("GetAllArticleAnalyses error = %v", err)
	}
	if len(resp.Analyses) != 2 {
		t.Errorf("len = %d, want 2", len(resp.Analyses))
	}
}

func TestListGlossaryEntriesEmpty(t *testing.T) {
	withTempStore(t)
	srv := NewAnalysisServer()
	resp, err := srv.ListGlossaryEntries(context.Background(), &protos.ListGlossaryEntriesRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListGlossaryEntries error = %v", err)
	}
	if len(resp.Entries) != 0 {
		t.Errorf("len = %d, want 0", len(resp.Entries))
	}
}

func TestListGlossaryEntriesReturnsSeededEntries(t *testing.T) {
	withTempStore(t)
	for _, e := range []*models.GlossaryEntry{
		{NormalizedKey: "apt", Term: "APT", Kind: models.GlossaryKindEntity, Definition: "def1"},
		{NormalizedKey: "ioc", Term: "IOC", Kind: models.GlossaryKindJargon, Definition: "def2"},
	} {
		if err := store.Db.UpsertGlossaryEntry(e); err != nil {
			t.Fatalf("UpsertGlossaryEntry error = %v", err)
		}
	}

	srv := NewAnalysisServer()
	resp, err := srv.ListGlossaryEntries(context.Background(), &protos.ListGlossaryEntriesRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListGlossaryEntries error = %v", err)
	}
	if len(resp.Entries) != 2 {
		t.Errorf("len = %d, want 2", len(resp.Entries))
	}
}

func TestSetGlossaryOverrideUpdatesEntry(t *testing.T) {
	withTempStore(t)
	// Seed an entry so the override has something to update.
	if err := store.Db.UpsertGlossaryEntry(&models.GlossaryEntry{
		NormalizedKey: "cobalt strike",
		Term:          "Cobalt Strike",
		Kind:          models.GlossaryKindEntity,
		Definition:    "a red-team tool",
	}); err != nil {
		t.Fatalf("UpsertGlossaryEntry error = %v", err)
	}

	srv := NewAnalysisServer()
	resp, err := srv.SetGlossaryOverride(context.Background(), &protos.SetGlossaryOverrideRequest{
		Term:       "Cobalt Strike",
		Definition: "curated: commercial C2 framework",
	})
	if err != nil {
		t.Fatalf("SetGlossaryOverride error = %v", err)
	}
	if resp.Entry == nil {
		t.Fatal("Entry = nil, want the updated entry")
	}
	if resp.Entry.EffectiveDefinition != "curated: commercial C2 framework" {
		t.Errorf("EffectiveDefinition = %q, want the override", resp.Entry.EffectiveDefinition)
	}
	if !resp.Entry.ManualOverride {
		t.Error("ManualOverride = false, want true")
	}
}
