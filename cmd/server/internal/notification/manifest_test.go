package notification

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"downlink/pkg/models"
)

func TestLoadManifestReadsExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ManifestFilename)
	want := Manifest{
		Version: ManifestVersion,
		Digests: []ManifestEntry{
			{Id: "digest-one", Filename: "downlink-digest-2026-04-24_1200.html"},
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
	if got.Version != ManifestVersion || len(got.Digests) != 1 || got.Digests[0].Id != "digest-one" {
		t.Fatalf("LoadManifest() = %+v", got)
	}
}

func TestLoadManifestBackfillsDigestFiles(t *testing.T) {
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
	if len(got.Digests) != 2 {
		t.Fatalf("backfilled digests = %+v, want 2", got.Digests)
	}
	if got.Digests[0].Filename != "downlink-digest-2026-04-25_0900.html" {
		t.Fatalf("first digest = %+v, want newest first", got.Digests[0])
	}
	if got.Digests[0].DisplayDate != "2026-04-25 09:00 UTC" {
		t.Fatalf("display date = %q", got.Digests[0].DisplayDate)
	}
}

func TestManifestUpsertReplacesByIDAndFilename(t *testing.T) {
	m := Manifest{Version: ManifestVersion}
	m.Upsert(ManifestEntry{
		Id:          "digest-one",
		Filename:    "downlink-digest-2026-04-24_1200.html",
		DisplayDate: "old",
	})
	m.Upsert(ManifestEntry{
		Id:          "digest-one",
		Filename:    "downlink-digest-2026-04-24_1200.html",
		DisplayDate: "new",
	})
	if len(m.Digests) != 1 || m.Digests[0].DisplayDate != "new" {
		t.Fatalf("upsert by id produced %+v", m.Digests)
	}

	m.Upsert(ManifestEntry{
		Id:          "digest-two",
		Filename:    "downlink-digest-2026-04-24_1200.html",
		DisplayDate: "filename replacement",
	})
	if len(m.Digests) != 1 || m.Digests[0].Id != "digest-two" {
		t.Fatalf("upsert by filename produced %+v", m.Digests)
	}
}

func TestManifestSortsNewestFirst(t *testing.T) {
	m := Manifest{Version: ManifestVersion}
	m.Upsert(ManifestEntry{Id: "old", Filename: "downlink-digest-2026-04-24_1200.html"})
	m.Upsert(ManifestEntry{Id: "new", Filename: "downlink-digest-2026-04-25_1200.html"})
	if got := m.Digests[0].Id; got != "new" {
		t.Fatalf("first digest = %q, want new", got)
	}
}

func TestArticleSetHashIsStable(t *testing.T) {
	a := models.Digest{
		Articles: []models.Article{{Id: "b"}, {Id: "a"}},
	}
	b := models.Digest{
		Articles: []models.Article{{Id: "a"}, {Id: "b"}},
	}
	if ArticleSetHash(a) != ArticleSetHash(b) {
		t.Fatalf("hash should be independent of article order")
	}
	c := models.Digest{
		Articles: []models.Article{{Id: "a"}, {Id: "c"}},
	}
	if ArticleSetHash(a) == ArticleSetHash(c) {
		t.Fatalf("hash should change when article set changes")
	}
}

func TestManifestEntryFromDigest(t *testing.T) {
	createdAt := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	digest := sampleDigest("digest-one", createdAt)

	got := ManifestEntryFromDigest(digest)
	if got.Id != "digest-one" ||
		got.Filename != "downlink-digest-2026-04-24_1200.html" ||
		got.DisplayDate != "2026-04-24 12:00 UTC" ||
		got.ProviderType != "openai" ||
		got.ModelName != "gpt-test" ||
		got.ArticleSetHash == "" {
		t.Fatalf("ManifestEntryFromDigest() = %+v", got)
	}
}
