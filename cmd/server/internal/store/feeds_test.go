package store

import (
	"errors"
	"testing"
	"time"

	"github.com/ma111e/downlink/pkg/models"
)

func TestStoreAndGetFeed(t *testing.T) {
	s := newTestStore(t)
	enabled := true
	want := models.Feed{Id: "f1", URL: "https://example.com/feed", Title: "Example", Enabled: &enabled}
	if err := s.StoreFeed(want); err != nil {
		t.Fatalf("StoreFeed() error = %v", err)
	}

	got, err := s.GetFeed("f1")
	if err != nil {
		t.Fatalf("GetFeed() error = %v", err)
	}
	if got.Id != "f1" || got.URL != want.URL || got.Title != "Example" {
		t.Fatalf("GetFeed() = %+v, want id/url/title to match %+v", got, want)
	}
}

func TestStoreFeedUpsertsOnSameID(t *testing.T) {
	s := newTestStore(t)
	if err := s.StoreFeed(models.Feed{Id: "f1", Title: "Old"}); err != nil {
		t.Fatalf("StoreFeed() error = %v", err)
	}
	if err := s.StoreFeed(models.Feed{Id: "f1", Title: "New"}); err != nil {
		t.Fatalf("StoreFeed() second error = %v", err)
	}

	feeds, err := s.ListFeeds()
	if err != nil {
		t.Fatalf("ListFeeds() error = %v", err)
	}
	if len(feeds) != 1 {
		t.Fatalf("ListFeeds() returned %d feeds, want 1 (same id must upsert)", len(feeds))
	}
	if feeds[0].Title != "New" {
		t.Fatalf("title = %q, want New (upsert overwrites)", feeds[0].Title)
	}
}

func TestGetFeedNotFound(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.GetFeed("ghost"); err == nil {
		t.Fatal("GetFeed(ghost) error = nil, want not-found error")
	}
}

func TestListFeedsPopulatesTopics(t *testing.T) {
	s := newTestStore(t)
	if err := s.StoreFeed(models.Feed{Id: "f1", Title: "A"}); err != nil {
		t.Fatalf("StoreFeed() error = %v", err)
	}
	if err := s.db.Create(&models.FeedTopic{FeedId: "f1", Topic: "zeta"}).Error; err != nil {
		t.Fatalf("seed topic error = %v", err)
	}
	if err := s.db.Create(&models.FeedTopic{FeedId: "f1", Topic: "alpha"}).Error; err != nil {
		t.Fatalf("seed topic error = %v", err)
	}

	feeds, err := s.ListFeeds()
	if err != nil {
		t.Fatalf("ListFeeds() error = %v", err)
	}
	if len(feeds) != 1 {
		t.Fatalf("got %d feeds, want 1", len(feeds))
	}
	// Topics are ordered ASC by the query.
	if len(feeds[0].Topics) != 2 || feeds[0].Topics[0] != "alpha" || feeds[0].Topics[1] != "zeta" {
		t.Fatalf("topics = %v, want [alpha zeta] in ASC order", feeds[0].Topics)
	}
}

func TestUpdateFeedLastFetch(t *testing.T) {
	s := newTestStore(t)
	if err := s.StoreFeed(models.Feed{Id: "f1"}); err != nil {
		t.Fatalf("StoreFeed() error = %v", err)
	}
	when := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	if err := s.UpdateFeedLastFetch("f1", when); err != nil {
		t.Fatalf("UpdateFeedLastFetch() error = %v", err)
	}
	got, err := s.GetFeed("f1")
	if err != nil {
		t.Fatalf("GetFeed() error = %v", err)
	}
	if !got.LastFetch.Equal(when) {
		t.Fatalf("LastFetch = %v, want %v", got.LastFetch, when)
	}
}

func TestDeleteFeedRemovesFeedAndTopics(t *testing.T) {
	s := newTestStore(t)
	if err := s.StoreFeed(models.Feed{Id: "f1"}); err != nil {
		t.Fatalf("StoreFeed() error = %v", err)
	}
	if err := s.db.Create(&models.FeedTopic{FeedId: "f1", Topic: "t"}).Error; err != nil {
		t.Fatalf("seed topic error = %v", err)
	}

	if err := s.DeleteFeed("f1"); err != nil {
		t.Fatalf("DeleteFeed() error = %v", err)
	}

	if _, err := s.GetFeed("f1"); err == nil {
		t.Error("feed still present after DeleteFeed")
	}
	// Topic rows are removed by an explicit DELETE in DeleteFeed (no FK cascade).
	var topicCount int64
	if err := s.db.Model(&models.FeedTopic{}).Where("feed_id = ?", "f1").Count(&topicCount).Error; err != nil {
		t.Fatalf("count topics error = %v", err)
	}
	if topicCount != 0 {
		t.Errorf("feed topics after delete = %d, want 0", topicCount)
	}
}

// TestDeleteFeedDoesNotCascadeArticles documents a discrepancy between the
// DeleteFeed doc-comment and reality: the comment claims articles are removed
// via "ON DELETE CASCADE", but Article.FeedId carries no FK constraint and
// migrations run with DisableForeignKeyConstraintWhenMigrating, so articles are
// in fact ORPHANED on feed delete. This test pins the current behavior; if
// DeleteFeed is fixed to also delete articles (or a real FK cascade is added),
// update this test to expect 0.
func TestDeleteFeedDoesNotCascadeArticles(t *testing.T) {
	s := newTestStore(t)
	if err := s.StoreFeed(models.Feed{Id: "f1"}); err != nil {
		t.Fatalf("StoreFeed() error = %v", err)
	}
	if err := s.StoreArticle(models.Article{Id: "a1", FeedId: "f1", Link: "https://x/1"}); err != nil {
		t.Fatalf("StoreArticle() error = %v", err)
	}

	if err := s.DeleteFeed("f1"); err != nil {
		t.Fatalf("DeleteFeed() error = %v", err)
	}

	var artCount int64
	if err := s.db.Model(&models.Article{}).Where("feed_id = ?", "f1").Count(&artCount).Error; err != nil {
		t.Fatalf("count articles error = %v", err)
	}
	if artCount != 1 {
		t.Fatalf("articles after delete = %d, want 1 (current behavior: no cascade); "+
			"if this changed, DeleteFeed cascade may now be implemented", artCount)
	}
}

// Sanity: errors from the store are wrapped, not the bare gorm sentinel.
func TestGetFeedErrorIsWrapped(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetFeed("ghost")
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, nil) {
		t.Fatal("unexpected nil-wrapped error")
	}
}
