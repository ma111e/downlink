package services

import (
	"context"
	"testing"
	"time"

	"github.com/ma111e/downlink/cmd/server/internal/store"
	"github.com/ma111e/downlink/pkg/models"
	"github.com/ma111e/downlink/pkg/protos"
)

// seedFeedAndArticle creates the minimal DB state (feed + article) needed by article service tests.
func seedFeedAndArticle(t *testing.T, feedId, articleId, title string) {
	t.Helper()
	if err := store.Db.StoreFeed(models.Feed{Id: feedId, URL: "https://example.com/feed", Title: "Test"}); err != nil {
		t.Fatalf("StoreFeed error = %v", err)
	}
	a := models.Article{
		Id:          articleId,
		FeedId:      feedId,
		Title:       title,
		Link:        "https://example.com/1",
		PublishedAt: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	}
	if err := store.Db.StoreArticle(a); err != nil {
		t.Fatalf("StoreArticle error = %v", err)
	}
}

func TestListArticlesEmpty(t *testing.T) {
	withTempStore(t)
	srv := NewArticleServer()
	resp, err := srv.ListArticles(context.Background(), &protos.ArticleFilter{})
	if err != nil {
		t.Fatalf("ListArticles error = %v", err)
	}
	if len(resp.Articles) != 0 {
		t.Errorf("len = %d, want 0", len(resp.Articles))
	}
}

func TestGetArticleNotFound(t *testing.T) {
	withTempStore(t)
	srv := NewArticleServer()
	if _, err := srv.GetArticle(context.Background(), &protos.GetArticleRequest{Id: "ghost"}); err == nil {
		t.Fatal("GetArticle(ghost) error = nil, want not-found")
	}
}

func TestGetArticleReturnsSeeded(t *testing.T) {
	withTempStore(t)
	seedFeedAndArticle(t, "f1", "art1", "Hello World")

	srv := NewArticleServer()
	got, err := srv.GetArticle(context.Background(), &protos.GetArticleRequest{Id: "art1"})
	if err != nil {
		t.Fatalf("GetArticle error = %v", err)
	}
	if got.Id != "art1" || got.Title != "Hello World" {
		t.Errorf("got %+v, want art1 / Hello World", got)
	}
}

func TestGetArticleCountsUnread(t *testing.T) {
	withTempStore(t)
	// Two feeds, two articles each.
	for _, feed := range []string{"f1", "f2"} {
		if err := store.Db.StoreFeed(models.Feed{Id: feed, URL: "https://" + feed + ".example.com/", Title: feed}); err != nil {
			t.Fatalf("StoreFeed(%s) error = %v", feed, err)
		}
	}
	for i, item := range []struct{ id, feed string }{
		{"a1", "f1"}, {"a2", "f1"}, {"a3", "f2"},
	} {
		pub := time.Date(2026, 6, 1, i, 0, 0, 0, time.UTC)
		if err := store.Db.StoreArticle(models.Article{
			Id: item.id, FeedId: item.feed, Title: item.id, Link: "https://x/" + item.id, PublishedAt: pub,
		}); err != nil {
			t.Fatalf("StoreArticle(%s) error = %v", item.id, err)
		}
	}

	srv := NewArticleServer()
	resp, err := srv.GetArticleCounts(context.Background(), &protos.ArticleFilter{})
	if err != nil {
		t.Fatalf("GetArticleCounts error = %v", err)
	}
	if resp.AllUnreadCount != 3 {
		t.Errorf("AllUnreadCount = %d, want 3", resp.AllUnreadCount)
	}
	if resp.UnreadByFeed["f1"] != 2 || resp.UnreadByFeed["f2"] != 1 {
		t.Errorf("UnreadByFeed = %v, want f1:2 f2:1", resp.UnreadByFeed)
	}
}

func TestMarkFeedArticlesReadClearsUnread(t *testing.T) {
	withTempStore(t)
	seedFeedAndArticle(t, "f1", "art1", "Article 1")
	seedFeedAndArticle(t, "f1", "art2", "Article 2")

	srv := NewArticleServer()
	if _, err := srv.MarkFeedArticlesRead(context.Background(), &protos.MarkFeedReadRequest{FeedId: "f1"}); err != nil {
		t.Fatalf("MarkFeedArticlesRead error = %v", err)
	}

	resp, err := srv.GetArticleCounts(context.Background(), &protos.ArticleFilter{FeedId: "f1"})
	if err != nil {
		t.Fatalf("GetArticleCounts error = %v", err)
	}
	if resp.AllUnreadCount != 0 {
		t.Errorf("AllUnreadCount = %d after MarkFeedArticlesRead, want 0", resp.AllUnreadCount)
	}
}
