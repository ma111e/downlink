package store

import (
	"path/filepath"
	"testing"

	"github.com/ma111e/downlink/pkg/models"
)

func newTestStore(t *testing.T) *GormStore {
	t.Helper()
	s, err := New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestUpsertGlossaryEntryDedup(t *testing.T) {
	s := newTestStore(t)

	first := &models.GlossaryEntry{NormalizedKey: "cobalt strike", Term: "Cobalt Strike", Kind: models.GlossaryKindEntity, Definition: "first def", TagId: "cobalt-strike"}
	if err := s.UpsertGlossaryEntry(first); err != nil {
		t.Fatalf("UpsertGlossaryEntry(first) error = %v", err)
	}
	if first.Id == "" {
		t.Fatal("expected created entry to have an Id")
	}

	// Same key, different definition: the first non-empty generated definition is sticky.
	second := &models.GlossaryEntry{NormalizedKey: "cobalt strike", Term: "cobalt-strike", Kind: models.GlossaryKindJargon, Definition: "second def"}
	if err := s.UpsertGlossaryEntry(second); err != nil {
		t.Fatalf("UpsertGlossaryEntry(second) error = %v", err)
	}
	if second.Id != first.Id {
		t.Errorf("expected upsert to resolve to existing Id %q, got %q", first.Id, second.Id)
	}

	got, err := s.GetGlossaryEntriesByKeys([]string{"cobalt strike"})
	if err != nil {
		t.Fatalf("GetGlossaryEntriesByKeys() error = %v", err)
	}
	e := got["cobalt strike"]
	if e == nil {
		t.Fatal("entry not found by key")
	}
	if e.Definition != "first def" {
		t.Errorf("definition = %q, want first def (sticky)", e.Definition)
	}
}

func TestUpsertGlossaryBackfillsMissingDefinition(t *testing.T) {
	s := newTestStore(t)

	// Entry created without a definition (e.g. should not normally happen, but exercise the path).
	if err := s.UpsertGlossaryEntry(&models.GlossaryEntry{NormalizedKey: "c2", Term: "C2", Kind: models.GlossaryKindEntity}); err != nil {
		t.Fatalf("UpsertGlossaryEntry() error = %v", err)
	}
	if err := s.UpsertGlossaryEntry(&models.GlossaryEntry{NormalizedKey: "c2", Term: "C2", Kind: models.GlossaryKindEntity, Definition: "command and control"}); err != nil {
		t.Fatalf("UpsertGlossaryEntry() backfill error = %v", err)
	}
	got, _ := s.GetGlossaryEntriesByKeys([]string{"c2"})
	if got["c2"].Definition != "command and control" {
		t.Errorf("definition = %q, want backfilled value", got["c2"].Definition)
	}
}

func TestManualOverrideNeverOverwritten(t *testing.T) {
	s := newTestStore(t)

	if err := s.UpsertGlossaryEntry(&models.GlossaryEntry{NormalizedKey: "phishing", Term: "phishing", Kind: models.GlossaryKindEntity, Definition: "auto def"}); err != nil {
		t.Fatalf("UpsertGlossaryEntry() error = %v", err)
	}
	if err := s.SetGlossaryManualOverride("Phishing", "human-written def"); err != nil {
		t.Fatalf("SetGlossaryManualOverride() error = %v", err)
	}

	// A later regeneration must not clobber the curated text.
	if err := s.UpsertGlossaryEntry(&models.GlossaryEntry{NormalizedKey: "phishing", Term: "phishing", Kind: models.GlossaryKindEntity, Definition: "different auto def"}); err != nil {
		t.Fatalf("UpsertGlossaryEntry() error = %v", err)
	}

	got, _ := s.GetGlossaryEntriesByKeys([]string{"phishing"})
	e := got["phishing"]
	if !e.ManualOverride || e.CuratedDefinition != "human-written def" {
		t.Errorf("override lost: manual=%v curated=%q", e.ManualOverride, e.CuratedDefinition)
	}
	if e.EffectiveDefinition() != "human-written def" {
		t.Errorf("EffectiveDefinition() = %q, want curated", e.EffectiveDefinition())
	}
}

func TestDigestGlossaryRoundTrip(t *testing.T) {
	s := newTestStore(t)

	entry := &models.GlossaryEntry{NormalizedKey: "lateral movement", Term: "lateral movement", Kind: models.GlossaryKindJargon, Definition: "moving deeper into a network after a breach"}
	if err := s.UpsertGlossaryEntry(entry); err != nil {
		t.Fatalf("UpsertGlossaryEntry() error = %v", err)
	}

	rows := []models.DigestGlossary{{DigestId: "digest-1", EntryId: entry.Id}}
	if err := s.StoreDigestGlossaryBatch(rows); err != nil {
		t.Fatalf("StoreDigestGlossaryBatch() error = %v", err)
	}
	// Idempotent: storing the same link again must not error.
	if err := s.StoreDigestGlossaryBatch(rows); err != nil {
		t.Fatalf("StoreDigestGlossaryBatch() repeat error = %v", err)
	}

	got, err := s.GetDigestGlossary("digest-1")
	if err != nil {
		t.Fatalf("GetDigestGlossary() error = %v", err)
	}
	if len(got) != 1 || got[0].Entry == nil || got[0].Entry.Term != "lateral movement" {
		t.Fatalf("GetDigestGlossary() = %+v, want one entry with preloaded term", got)
	}
}
