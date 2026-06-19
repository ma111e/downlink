package store

import (
	"fmt"
	"testing"
	"time"

	"github.com/ma111e/downlink/pkg/models"
)

// seedArticles inserts n articles in a single feed, all published within the
// given day, and returns the feed id and the window covering them.
func seedArticles(t *testing.T, s *GormStore, n int) (start, end time.Time) {
	t.Helper()

	feedID := "feed-1"
	if err := s.StoreFeed(models.Feed{Id: feedID, URL: "https://example.com/feed", Title: "Example"}); err != nil {
		t.Fatalf("StoreFeed() error = %v", err)
	}

	base := time.Date(2026, 6, 18, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		// Spread within the day so ordering is well-defined.
		pub := base.Add(time.Duration(i) * time.Minute)
		a := models.Article{
			Id:          fmt.Sprintf("art-%03d", i),
			FeedId:      feedID,
			Title:       fmt.Sprintf("Article %d", i),
			Link:        fmt.Sprintf("https://example.com/%d", i),
			PublishedAt: pub,
		}
		if err := s.StoreArticle(a); err != nil {
			t.Fatalf("StoreArticle(%d) error = %v", i, err)
		}
	}

	return base.Add(-time.Hour), base.Add(24 * time.Hour)
}

// The digest fetch sets Unbounded so it pulls every article in the window,
// not just the default first page. Without it the result is capped at 30.
func TestListArticlesUnboundedReturnsAll(t *testing.T) {
	s := newTestStore(t)
	const total = 120
	start, end := seedArticles(t, s, total)

	got, err := s.ListArticles(models.ArticleFilter{StartDate: &start, EndDate: &end, Unbounded: true})
	if err != nil {
		t.Fatalf("ListArticles(unbounded) error = %v", err)
	}
	if len(got) != total {
		t.Errorf("unbounded returned %d articles, want %d", len(got), total)
	}
}

// The default (UI) path must keep its 30 default / 50 cap behavior.
func TestListArticlesBoundedKeepsCap(t *testing.T) {
	s := newTestStore(t)
	start, end := seedArticles(t, s, 120)

	def, err := s.ListArticles(models.ArticleFilter{StartDate: &start, EndDate: &end})
	if err != nil {
		t.Fatalf("ListArticles(default) error = %v", err)
	}
	if len(def) != 30 {
		t.Errorf("default returned %d articles, want 30", len(def))
	}

	capped, err := s.ListArticles(models.ArticleFilter{StartDate: &start, EndDate: &end, Limit: 1000})
	if err != nil {
		t.Fatalf("ListArticles(limit=1000) error = %v", err)
	}
	if len(capped) != 50 {
		t.Errorf("oversized limit returned %d articles, want cap of 50", len(capped))
	}
}
