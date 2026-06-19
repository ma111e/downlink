package feedserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ma111e/downlink/cmd/server/internal/store"
	"github.com/ma111e/downlink/pkg/models"
)

// fakeStore implements just the store.Store methods the feed server uses; the
// rest are inherited from the embedded interface and panic if ever called.
type fakeStore struct {
	store.Store
	feeds    []models.Feed
	articles []models.Article
}

func (f *fakeStore) ListFeeds() ([]models.Feed, error) { return f.feeds, nil }

func (f *fakeStore) ListArticles(_ models.ArticleFilter) ([]models.Article, error) {
	return f.articles, nil
}

func newTestServer(baseURL string) *FeedServer {
	fs := &fakeStore{
		feeds: []models.Feed{
			{Id: "f1", URL: "https://source.example/rss.xml", Title: "My Feed", LastFetch: time.Unix(0, 0)},
		},
		articles: []models.Article{
			{Id: "a1", FeedId: "f1", Title: "Absolute", Link: "https://source.example/posts/1", PublishedAt: time.Unix(0, 0)},
			{Id: "a2", FeedId: "f1", Title: "Relative", Link: "/posts/2", PublishedAt: time.Unix(0, 0)},
		},
	}
	return NewFeedServer(fs, 0, baseURL)
}

func TestHandleIndexLinks(t *testing.T) {
	t.Run("absolute with base url", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		newTestServer("https://feeds.example.com").handleIndex(rec, req)

		body := rec.Body.String()
		if !strings.Contains(body, `href="https://feeds.example.com/feeds/my-feed"`) {
			t.Errorf("index missing absolute feed link, got:\n%s", body)
		}
	})

	t.Run("relative without base url", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		newTestServer("").handleIndex(rec, req)

		body := rec.Body.String()
		if !strings.Contains(body, `href="/feeds/my-feed"`) {
			t.Errorf("index missing relative feed link, got:\n%s", body)
		}
	})
}

func TestHandleFeedRequestLinks(t *testing.T) {
	t.Run("absolute self link and relative article resolved", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/feeds/my-feed", nil)
		newTestServer("https://feeds.example.com").handleFeedRequest(rec, req)

		body := rec.Body.String()
		if !strings.Contains(body, "https://feeds.example.com/feeds/my-feed") {
			t.Errorf("atom missing absolute self link, got:\n%s", body)
		}
		if !strings.Contains(body, "https://feeds.example.com/posts/2") {
			t.Errorf("relative article link not resolved against base url, got:\n%s", body)
		}
		if !strings.Contains(body, "https://source.example/posts/1") {
			t.Errorf("absolute article link should pass through, got:\n%s", body)
		}
	})

	t.Run("empty base keeps article links untouched", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/feeds/my-feed", nil)
		newTestServer("").handleFeedRequest(rec, req)

		body := rec.Body.String()
		if !strings.Contains(body, "https://source.example/posts/1") {
			t.Errorf("absolute article link missing, got:\n%s", body)
		}
		// The relative source link stays relative (no base prepended).
		if strings.Contains(body, "https://feeds.example.com") {
			t.Errorf("no base url should mean no absolute feed host, got:\n%s", body)
		}
	})
}
