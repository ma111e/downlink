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
	profiles     []models.Profile
	profileFeeds map[string][]models.Feed
	articles     []models.Article
}

func (f *fakeStore) ListProfiles() ([]models.Profile, error) { return f.profiles, nil }

func (f *fakeStore) ListProfileFeeds(profileID string) ([]models.Feed, error) {
	return f.profileFeeds[profileID], nil
}

func (f *fakeStore) ListArticles(_ models.ArticleFilter) ([]models.Article, error) {
	return f.articles, nil
}

func newTestServer(baseURL string) *FeedServer {
	feed := models.Feed{Id: "f1", URL: "https://source.example/rss.xml", Title: "My Feed", LastFetch: time.Unix(0, 0)}
	st := &fakeStore{
		profiles: []models.Profile{{Id: "infosec", Name: "Infosec Daily"}},
		profileFeeds: map[string][]models.Feed{
			"infosec": {feed},
		},
		articles: []models.Article{
			{Id: "a1", FeedId: "f1", Title: "Absolute", Link: "https://source.example/posts/1", PublishedAt: time.Unix(0, 0)},
			{Id: "a2", FeedId: "f1", Title: "Relative", Link: "/posts/2", PublishedAt: time.Unix(0, 0)},
		},
	}
	return NewFeedServer(st, 0, baseURL)
}

func TestHandleProfileIndex(t *testing.T) {
	t.Run("absolute with base url", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		newTestServer("https://feeds.example.com").handleProfileIndex(rec, req)

		body := rec.Body.String()
		if !strings.Contains(body, `href="https://feeds.example.com/infosec/"`) {
			t.Errorf("index missing absolute profile link, got:\n%s", body)
		}
		if !strings.Contains(body, "Infosec Daily") {
			t.Errorf("index missing profile name, got:\n%s", body)
		}
	})

	t.Run("relative without base url", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		newTestServer("").handleProfileIndex(rec, req)

		if body := rec.Body.String(); !strings.Contains(body, `href="/infosec/"`) {
			t.Errorf("index missing relative profile link, got:\n%s", body)
		}
	})
}

func TestHandleProfileFeeds(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/infosec/", nil)
	req.SetPathValue("profile", "infosec")
	newTestServer("https://feeds.example.com").handleProfileFeeds(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, `href="https://feeds.example.com/infosec/feeds/my-feed"`) {
		t.Errorf("profile feed list missing scoped feed link, got:\n%s", body)
	}
}

func TestHandleProfileFeed(t *testing.T) {
	t.Run("absolute self link and relative article resolved", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/infosec/feeds/my-feed", nil)
		req.SetPathValue("profile", "infosec")
		req.SetPathValue("name", "my-feed")
		newTestServer("https://feeds.example.com").handleProfileFeed(rec, req)

		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/atom+xml") {
			t.Errorf("content-type = %q, want atom", ct)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "https://feeds.example.com/infosec/feeds/my-feed") {
			t.Errorf("atom missing absolute self link, got:\n%s", body)
		}
		if !strings.Contains(body, "https://feeds.example.com/posts/2") {
			t.Errorf("relative article link not resolved against base url, got:\n%s", body)
		}
		if !strings.Contains(body, "https://source.example/posts/1") {
			t.Errorf("absolute article link should pass through, got:\n%s", body)
		}
	})

	t.Run("unknown feed is 404", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/infosec/feeds/nope", nil)
		req.SetPathValue("profile", "infosec")
		req.SetPathValue("name", "nope")
		newTestServer("").handleProfileFeed(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", rec.Code)
		}
	})
}
