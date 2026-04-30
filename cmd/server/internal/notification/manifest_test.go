package notification

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadManifestReadsExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ManifestFilename)
	want := Manifest{
		GeneratedAt: "2026-04-24 12:01 UTC",
		SourceRepo:  "downlink",
		Digests: []ManifestEntry{
			{
				Filename:     "downlink-digest-2026-04-24_1200.html",
				StartedAt:    "2026-04-24 12:00 UTC",
				TimeWindow:   "24h",
				ArticleCount: 2,
				MustCount:    1,
				ShouldCount:  1,
				Provider:     "openai",
				Model:        "gpt-test",
				Headlines:    []string{"Article A"},
				Summary:      "A short digest.",
			},
		},
	}
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}
	if got.SourceRepo != "downlink" || got.GeneratedAt != "2026-04-24 12:01 UTC" || len(got.Digests) != 1 || got.Digests[0].Provider != "openai" {
		t.Fatalf("LoadManifest() = %+v", got)
	}
}

func TestLoadManifestMissingReturnsEmptyCurrentManifest(t *testing.T) {
	dir := t.TempDir()
	files := []string{
		"downlink-digest-2026-04-24_1200.html",
		"downlink-digest-2026-04-25_0900.html",
		"index.html",
		"notes.txt",
	}
	for _, file := range files {
		if err := os.WriteFile(filepath.Join(dir, file), []byte("x"), 0644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", file, err)
		}
	}

	got, err := LoadManifest(filepath.Join(dir, ManifestFilename))
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}
	if got.SourceRepo != "downlink" || len(got.Digests) != 0 {
		t.Fatalf("LoadManifest() = %+v, want empty current manifest without backfill", got)
	}
}

func TestLoadManifestDoesNotCarryOldSchemaEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ManifestFilename)
	oldSchema := map[string]any{
		"version":   1,
		"updatedAt": "2026-04-24T12:00:00Z",
		"digests": []map[string]any{
			{
				"filename":    "downlink-digest-2026-04-24_1200.html",
				"displayDate": "2026-04-24 12:00 UTC",
			},
		},
	}
	data, err := json.Marshal(oldSchema)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}
	if got.SourceRepo != "downlink" || len(got.Digests) != 0 {
		t.Fatalf("LoadManifest() = %+v, want old schema discarded", got)
	}
}

func TestManifestUpsertReplacesByFilename(t *testing.T) {
	m := Manifest{SourceRepo: "downlink"}
	m.Upsert(ManifestEntry{
		Filename:  "downlink-digest-2026-04-24_1200.html",
		StartedAt: "old",
	})
	m.Upsert(ManifestEntry{
		Filename:  "downlink-digest-2026-04-24_1200.html",
		StartedAt: "new",
	})
	if len(m.Digests) != 1 || m.Digests[0].StartedAt != "new" {
		t.Fatalf("upsert by filename produced %+v", m.Digests)
	}
}

func TestManifestSortsNewestFirst(t *testing.T) {
	m := Manifest{SourceRepo: "downlink"}
	m.Upsert(ManifestEntry{Filename: "downlink-digest-2026-04-24_1200.html", StartedAt: "old"})
	m.Upsert(ManifestEntry{Filename: "downlink-digest-2026-04-25_1200.html", StartedAt: "new"})
	if got := m.Digests[0].StartedAt; got != "new" {
		t.Fatalf("first digest = %q, want new", got)
	}
}

func TestManifestEntryFromDigest(t *testing.T) {
	createdAt := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	digest := sampleDigest("digest-one", createdAt)

	got := ManifestEntryFromDigest(digest)
	if got.Filename != "downlink-digest-2026-04-24_1200.html" ||
		got.StartedAt != "2026-04-24 12:00 UTC" ||
		got.TimeWindow != "1 day" ||
		got.ArticleCount != 2 ||
		got.MustCount != 1 ||
		got.ShouldCount != 1 ||
		got.MayCount != 0 ||
		got.OptCount != 0 ||
		got.Provider != "openai" ||
		got.Model != "gpt-test" ||
		got.Summary != "A short digest." {
		t.Fatalf("ManifestEntryFromDigest() = %+v", got)
	}
	if len(got.Headlines) != 2 || got.Headlines[0] != "Article B" || got.Headlines[1] != "Article A" {
		t.Fatalf("headlines = %+v", got.Headlines)
	}
}

func TestManifestWriteSetsGeneratedAtAndSourceRepo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ManifestFilename)
	m := Manifest{Digests: []ManifestEntry{{Filename: "downlink-digest-2026-04-24_1200.html"}}}
	if err := m.Write(path); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	var got Manifest
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got.SourceRepo != "downlink" || got.GeneratedAt == "" {
		t.Fatalf("written manifest = %+v", got)
	}
}
