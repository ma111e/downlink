package store

import (
	"path/filepath"
	"testing"
	"time"

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

func TestBackfillGlossaryKeys(t *testing.T) {
	s := newTestStore(t)

	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)

	// a and b have OLD keys that differ only by punctuation; both collapse to "wscript exe".
	// c already has a clean key and must be left untouched.
	a := &models.GlossaryEntry{Id: "a", NormalizedKey: "wscript.exe", Term: "wscript.exe", Kind: models.GlossaryKindEntity, Definition: "keeper def", CreatedAt: older}
	b := &models.GlossaryEntry{Id: "b", NormalizedKey: "wscript-exe", Term: "wscript.exe", Kind: models.GlossaryKindEntity, Definition: "loser def", CreatedAt: newer}
	c := &models.GlossaryEntry{Id: "c", NormalizedKey: "cobalt strike", Term: "Cobalt Strike", Kind: models.GlossaryKindEntity, Definition: "cs", CreatedAt: older}
	for _, e := range []*models.GlossaryEntry{a, b, c} {
		if err := s.db.Create(e).Error; err != nil {
			t.Fatalf("seed glossary entry error: %v", err)
		}
	}

	// d1 references both a and b (so merging b→a hits the composite-PK conflict and is skipped),
	// d2 references only b (moves to a), d3 references c.
	for _, r := range []models.DigestGlossary{
		{DigestId: "d1", EntryId: "a"},
		{DigestId: "d1", EntryId: "b"},
		{DigestId: "d2", EntryId: "b"},
		{DigestId: "d3", EntryId: "c"},
	} {
		r := r
		if err := s.db.Create(&r).Error; err != nil {
			t.Fatalf("seed digest glossary error: %v", err)
		}
	}

	if err := s.backfillGlossaryKeys(); err != nil {
		t.Fatalf("backfillGlossaryKeys() error = %v", err)
	}

	var entries []models.GlossaryEntry
	if err := s.db.Find(&entries).Error; err != nil {
		t.Fatalf("load entries: %v", err)
	}
	byId := map[string]models.GlossaryEntry{}
	for _, e := range entries {
		byId[e.Id] = e
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries after merge, got %d: %+v", len(entries), entries)
	}
	if _, ok := byId["b"]; ok {
		t.Error("loser entry b should have been deleted")
	}
	if byId["a"].NormalizedKey != "wscript exe" {
		t.Errorf("keeper key = %q, want 'wscript exe'", byId["a"].NormalizedKey)
	}
	if byId["a"].Definition != "keeper def" {
		t.Errorf("keeper kept wrong definition %q (oldest should win)", byId["a"].Definition)
	}
	if byId["c"].NormalizedKey != "cobalt strike" {
		t.Errorf("untouched key = %q, want 'cobalt strike'", byId["c"].NormalizedKey)
	}

	var dg []models.DigestGlossary
	if err := s.db.Find(&dg).Error; err != nil {
		t.Fatalf("load digest glossary: %v", err)
	}
	got := map[string]string{}
	for _, r := range dg {
		got[r.DigestId] = r.EntryId
	}
	if len(dg) != 3 || got["d1"] != "a" || got["d2"] != "a" || got["d3"] != "c" {
		t.Errorf("digest references not repointed correctly: %+v", dg)
	}

	// Idempotent: a second run is a no-op.
	if err := s.backfillGlossaryKeys(); err != nil {
		t.Fatalf("second backfillGlossaryKeys() error = %v", err)
	}
	var n int64
	s.db.Model(&models.GlossaryEntry{}).Count(&n)
	if n != 2 {
		t.Errorf("idempotent run changed entry count to %d", n)
	}
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

func TestListGlossaryEntries(t *testing.T) {
	s := newTestStore(t)

	for _, e := range []*models.GlossaryEntry{
		{NormalizedKey: "ransomware", Term: "ransomware", Kind: models.GlossaryKindJargon, Category: "concept", Definition: "Malware that locks files for ransom."},
		{NormalizedKey: "cobalt strike", Term: "Cobalt Strike", Kind: models.GlossaryKindEntity, Category: "tool", Definition: "A commercial hacking toolkit."},
		{NormalizedKey: "c2", Term: "C2", Kind: models.GlossaryKindJargon, Category: "concept", Definition: "Command and control."},
	} {
		if err := s.UpsertGlossaryEntry(e); err != nil {
			t.Fatalf("UpsertGlossaryEntry() error = %v", err)
		}
	}

	all, err := s.ListGlossaryEntries(0)
	if err != nil {
		t.Fatalf("ListGlossaryEntries() error = %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(all))
	}
	// Ordered by term, case-insensitive: C2, Cobalt Strike, ransomware.
	if all[0].Term != "C2" || all[1].Term != "Cobalt Strike" || all[2].Term != "ransomware" {
		t.Errorf("unexpected order: %q, %q, %q", all[0].Term, all[1].Term, all[2].Term)
	}

	limited, err := s.ListGlossaryEntries(2)
	if err != nil {
		t.Fatalf("ListGlossaryEntries(2) error = %v", err)
	}
	if len(limited) != 2 {
		t.Errorf("expected 2 entries with limit, got %d", len(limited))
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
