package store

import (
	"testing"
	"time"

	"github.com/ma111e/downlink/pkg/models"
)

func intPtr(n int) *int { return &n }

// seedArticle inserts a bare article (feed created on demand) for digest tests.
func seedArticle(t *testing.T, s *GormStore, feedID, artID string) {
	t.Helper()
	if _, err := s.GetFeed(feedID); err != nil {
		if err := s.StoreFeed(models.Feed{Id: feedID}); err != nil {
			t.Fatalf("StoreFeed() error = %v", err)
		}
	}
	if err := s.StoreArticle(models.Article{Id: artID, FeedId: feedID, Link: "https://x/" + artID}); err != nil {
		t.Fatalf("StoreArticle(%s) error = %v", artID, err)
	}
}

func TestStoreAndGetDigest(t *testing.T) {
	s := newTestStore(t)
	d := models.Digest{Id: "d1", Title: "Morning", CreatedAt: time.Now(), ArticleCount: intPtr(0)}
	if err := s.StoreDigest(d); err != nil {
		t.Fatalf("StoreDigest() error = %v", err)
	}
	got, err := s.GetDigest("d1")
	if err != nil {
		t.Fatalf("GetDigest() error = %v", err)
	}
	if got.Id != "d1" || got.Title != "Morning" {
		t.Fatalf("GetDigest() = %+v, want id d1 title Morning", got)
	}
}

func TestGetDigestNotFound(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.GetDigest("ghost"); err == nil {
		t.Fatal("GetDigest(ghost) error = nil, want not-found")
	}
}

func TestListDigestsOrderAndLimit(t *testing.T) {
	s := newTestStore(t)
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	// Insert out of chronological order to prove ordering is by created_at DESC.
	for _, d := range []models.Digest{
		{Id: "old", CreatedAt: base, ArticleCount: intPtr(0)},
		{Id: "new", CreatedAt: base.Add(2 * time.Hour), ArticleCount: intPtr(0)},
		{Id: "mid", CreatedAt: base.Add(time.Hour), ArticleCount: intPtr(0)},
	} {
		if err := s.StoreDigest(d); err != nil {
			t.Fatalf("StoreDigest() error = %v", err)
		}
	}

	all, err := s.ListDigests(0, false)
	if err != nil {
		t.Fatalf("ListDigests() error = %v", err)
	}
	if len(all) != 3 || all[0].Id != "new" || all[1].Id != "mid" || all[2].Id != "old" {
		t.Fatalf("order = %v, want [new mid old] (created_at DESC)", ids(all))
	}

	limited, err := s.ListDigests(2, false)
	if err != nil {
		t.Fatalf("ListDigests(2) error = %v", err)
	}
	if len(limited) != 2 || limited[0].Id != "new" || limited[1].Id != "mid" {
		t.Fatalf("limited = %v, want [new mid]", ids(limited))
	}
}

func ids(ds []models.Digest) []string {
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = d.Id
	}
	return out
}

func TestStoreDigestArticleIncrementsCountAndValidates(t *testing.T) {
	s := newTestStore(t)
	if err := s.StoreDigest(models.Digest{Id: "d1", ArticleCount: intPtr(0)}); err != nil {
		t.Fatalf("StoreDigest() error = %v", err)
	}
	seedArticle(t, s, "f1", "a1")

	if err := s.StoreDigestArticle("d1", "a1"); err != nil {
		t.Fatalf("StoreDigestArticle() error = %v", err)
	}
	got, _ := s.GetDigest("d1")
	if got.ArticleCount == nil || *got.ArticleCount != 1 {
		t.Fatalf("ArticleCount = %v, want 1", got.ArticleCount)
	}

	if err := s.StoreDigestArticle("ghost", "a1"); err == nil {
		t.Error("StoreDigestArticle with unknown digest = nil, want error")
	}
	if err := s.StoreDigestArticle("d1", "ghost"); err == nil {
		t.Error("StoreDigestArticle with unknown article = nil, want error")
	}
}

func TestStoreDigestArticlesBatch(t *testing.T) {
	s := newTestStore(t)
	if err := s.StoreDigest(models.Digest{Id: "d1", ArticleCount: intPtr(0)}); err != nil {
		t.Fatalf("StoreDigest() error = %v", err)
	}
	seedArticle(t, s, "f1", "a1")
	seedArticle(t, s, "f1", "a2")

	if err := s.StoreDigestArticlesBatch("d1", []string{"a1", "a2"}); err != nil {
		t.Fatalf("StoreDigestArticlesBatch() error = %v", err)
	}
	got := s.mustArticles(t, "d1")
	if len(got) != 2 {
		t.Fatalf("associated %d articles, want 2", len(got))
	}
	d, _ := s.GetDigest("d1")
	if *d.ArticleCount != 2 {
		t.Fatalf("ArticleCount = %d, want 2", *d.ArticleCount)
	}

	// Empty batch is a no-op (no error, count unchanged).
	if err := s.StoreDigestArticlesBatch("d1", nil); err != nil {
		t.Fatalf("empty batch error = %v", err)
	}
}

func (s *GormStore) mustArticles(t *testing.T, digestID string) []models.Article {
	t.Helper()
	arts, err := s.GetDigestArticles(digestID)
	if err != nil {
		t.Fatalf("GetDigestArticles() error = %v", err)
	}
	return arts
}

func TestFindDigestsWithSameArticleSet(t *testing.T) {
	s := newTestStore(t)
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	for _, id := range []string{"d1", "d2", "d3"} {
		if err := s.StoreDigest(models.Digest{Id: id, CreatedAt: base, ArticleCount: intPtr(0)}); err != nil {
			t.Fatalf("StoreDigest(%s) error = %v", id, err)
		}
	}
	for _, a := range []string{"a1", "a2", "a3"} {
		seedArticle(t, s, "f1", a)
	}

	// d1 and d2 share the exact set {a1,a2}; d3 has {a1,a2,a3} (superset).
	if err := s.StoreDigestArticlesBatch("d1", []string{"a1", "a2"}); err != nil {
		t.Fatal(err)
	}
	if err := s.StoreDigestArticlesBatch("d2", []string{"a1", "a2"}); err != nil {
		t.Fatal(err)
	}
	if err := s.StoreDigestArticlesBatch("d3", []string{"a1", "a2", "a3"}); err != nil {
		t.Fatal(err)
	}

	siblings, err := s.FindDigestsWithSameArticleSet("d1")
	if err != nil {
		t.Fatalf("FindDigestsWithSameArticleSet() error = %v", err)
	}
	got := map[string]bool{}
	for _, d := range siblings {
		got[d.Id] = true
	}
	if !got["d1"] || !got["d2"] {
		t.Fatalf("siblings = %v, want d1 and d2 (identical set)", ids(siblings))
	}
	if got["d3"] {
		t.Fatalf("siblings = %v, want d3 EXCLUDED (superset, not equal)", ids(siblings))
	}
	if len(siblings) != 2 {
		t.Fatalf("got %d siblings, want exactly 2", len(siblings))
	}
}
