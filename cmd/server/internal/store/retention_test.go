package store

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/ma111e/downlink/pkg/models"
)

// TestZstdRoundTrip covers the shared codec: empty in -> empty out, and a large
// repetitive payload survives a compress/decompress cycle.
func TestZstdRoundTrip(t *testing.T) {
	if got := compressString(""); got != nil {
		t.Errorf("compressString(\"\") = %v, want nil", got)
	}
	if s, err := decompressBytes(nil); err != nil || s != "" {
		t.Errorf("decompressBytes(nil) = %q, %v; want \"\", nil", s, err)
	}

	payload := strings.Repeat("the quick brown fox <html>&amp;</html> ", 5000)
	blob := compressString(payload)
	if len(blob) >= len(payload) {
		t.Errorf("compressed size %d not smaller than input %d", len(blob), len(payload))
	}
	got, err := decompressBytes(blob)
	if err != nil {
		t.Fatalf("decompressBytes error = %v", err)
	}
	if got != payload {
		t.Errorf("round-trip mismatch: got %d bytes, want %d", len(got), len(payload))
	}
}

// TestAnalysisSerializerRoundTrip verifies the serializer-tagged blob columns are
// stored compressed yet read back as the original plaintext through GORM.
func TestAnalysisSerializerRoundTrip(t *testing.T) {
	s := newTestStore(t)
	big := strings.Repeat("comprehensive synthesis body ", 2000)
	a := &models.ArticleAnalysis{
		Id:                     "an1",
		ArticleId:              "art1",
		ProfileId:              "p1",
		ComprehensiveSynthesis: big,
		RawResponse:            "raw json {}",
		CreatedAt:              time.Now(),
	}
	if err := s.SaveArticleAnalysis(a); err != nil {
		t.Fatalf("SaveArticleAnalysis error = %v", err)
	}

	got, err := s.GetArticleAnalysis("art1", "p1")
	if err != nil {
		t.Fatalf("GetArticleAnalysis error = %v", err)
	}
	if got.ComprehensiveSynthesis != big {
		t.Errorf("ComprehensiveSynthesis round-trip mismatch (got %d bytes)", len(got.ComprehensiveSynthesis))
	}
	if got.RawResponse != "raw json {}" {
		t.Errorf("RawResponse = %q, want %q", got.RawResponse, "raw json {}")
	}

	// The stored column must be compressed bytes (zstd magic), not the plaintext.
	var raw []byte
	if err := s.db.Raw(`SELECT comprehensive_synthesis FROM article_analyses WHERE id = ?`, "an1").Row().Scan(&raw); err != nil {
		t.Fatalf("read raw column error = %v", err)
	}
	if len(raw) >= len(big) {
		t.Errorf("stored blob %d not smaller than plaintext %d — serializer not compressing", len(raw), len(big))
	}
	if !bytes.HasPrefix(raw, []byte{0x28, 0xb5, 0x2f, 0xfd}) {
		t.Errorf("stored blob missing zstd magic; serializer tag likely absent: % x", raw[:4])
	}
}

// TestFreshDBUsesFullAutoVacuum guards the pragma ordering: a fresh DB must adopt
// full auto-vacuum (mode 1) immediately, so the file shrinks on its own.
func TestFreshDBUsesFullAutoVacuum(t *testing.T) {
	s := newTestStore(t)
	var mode int
	if err := s.db.Raw("PRAGMA auto_vacuum").Row().Scan(&mode); err != nil {
		t.Fatalf("read auto_vacuum: %v", err)
	}
	if mode != 1 {
		t.Errorf("auto_vacuum = %d, want 1 (FULL) on a fresh DB", mode)
	}
}

// TestPruneAnalyses keeps the latest N per (article, profile) and never deletes a
// digest-referenced analysis.
func TestPruneAnalyses(t *testing.T) {
	s := newTestStore(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// art1/p1: 4 analyses (a1 oldest .. a4 newest). art1/p2: 1 analysis (isolated group).
	mk := func(id, art, prof string, n int) {
		a := &models.ArticleAnalysis{Id: id, ArticleId: art, ProfileId: prof, CreatedAt: base.Add(time.Duration(n) * time.Hour)}
		if err := s.SaveArticleAnalysis(a); err != nil {
			t.Fatalf("save %s: %v", id, err)
		}
	}
	mk("a1", "art1", "p1", 1)
	mk("a2", "art1", "p1", 2)
	mk("a3", "art1", "p1", 3)
	mk("a4", "art1", "p1", 4)
	mk("b1", "art1", "p2", 1)

	// A digest references the oldest analysis a1, which must survive pruning.
	if err := s.db.Exec(`INSERT INTO digest_analyses (digest_id, analysis_id, article_id) VALUES (?, ?, ?)`, "d1", "a1", "art1").Error; err != nil {
		t.Fatalf("insert digest_analyses: %v", err)
	}

	if err := s.PruneAnalyses(2); err != nil {
		t.Fatalf("PruneAnalyses error = %v", err)
	}

	remaining := map[string]bool{}
	var ids []string
	if err := s.db.Model(&models.ArticleAnalysis{}).Pluck("id", &ids).Error; err != nil {
		t.Fatalf("pluck ids: %v", err)
	}
	for _, id := range ids {
		remaining[id] = true
	}

	// keep=2 -> a3,a4 kept; a2 pruned; a1 kept (digest-referenced); b1 kept (own group).
	wantKept := []string{"a1", "a3", "a4", "b1"}
	wantGone := []string{"a2"}
	for _, id := range wantKept {
		if !remaining[id] {
			t.Errorf("analysis %s was pruned but should be kept", id)
		}
	}
	for _, id := range wantGone {
		if remaining[id] {
			t.Errorf("analysis %s survived but should be pruned", id)
		}
	}
}

// TestDeleteUnusedTags removes tags with no article_tags rows and keeps referenced ones.
func TestDeleteUnusedTags(t *testing.T) {
	s := newTestStore(t)
	used := &models.Tag{Id: "used", Name: "used"}
	orphan := &models.Tag{Id: "orphan", Name: "orphan"}
	if err := s.db.Create(used).Error; err != nil {
		t.Fatalf("create used: %v", err)
	}
	if err := s.db.Create(orphan).Error; err != nil {
		t.Fatalf("create orphan: %v", err)
	}
	if err := s.db.Exec(`INSERT INTO article_tags (article_id, tag_id) VALUES (?, ?)`, "art1", "used").Error; err != nil {
		t.Fatalf("insert article_tags: %v", err)
	}

	if err := s.DeleteUnusedTags(); err != nil {
		t.Fatalf("DeleteUnusedTags error = %v", err)
	}

	var count int64
	if err := s.db.Model(&models.Tag{}).Where("id = ?", "orphan").Count(&count).Error; err != nil {
		t.Fatalf("count orphan: %v", err)
	}
	if count != 0 {
		t.Errorf("orphan tag survived DeleteUnusedTags")
	}
	if err := s.db.Model(&models.Tag{}).Where("id = ?", "used").Count(&count).Error; err != nil {
		t.Fatalf("count used: %v", err)
	}
	if count != 1 {
		t.Errorf("used tag was deleted, want kept")
	}
}
