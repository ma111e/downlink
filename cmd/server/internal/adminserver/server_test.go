package adminserver

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ma111e/downlink/cmd/server/internal/store"
)

// TestRefreshHandlersEndToEnd exercises the full route → store → template path
// against a real (temp) store, without binding a port.
func TestRefreshHandlersEndToEnd(t *testing.T) {
	s, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	if err := s.StartFeedRefreshRun("refresh-e2e", "manual-all", time.Now()); err != nil {
		t.Fatalf("StartFeedRefreshRun() error = %v", err)
	}
	_ = s.RecordFeedRefresh(store.FeedRefreshInput{RunID: "refresh-e2e", FeedId: "feed-1", FeedTitle: "Hacker News", FeedURL: "https://news.ycombinator.com/rss", Success: true, TotalFetched: 40, Stored: 5, Skipped: 33, RawBody: []byte("<rss>RAWFEEDBYTES</rss>"), RawStatus: 200, RawType: "application/rss+xml"})
	_ = s.RecordFeedRefresh(store.FeedRefreshInput{RunID: "refresh-e2e", FeedId: "feed-2", FeedTitle: "Broken", Success: false, FetchError: "failed to fetch feed: 503"})
	_ = s.FinishFeedRefreshRun("refresh-e2e", time.Now())

	a := NewAdminServer(s, 0)

	// List page.
	rec := httptest.NewRecorder()
	a.handleRefreshRuns(rec, httptest.NewRequest(http.MethodGet, "/feeds", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /feeds status = %d, want 200", rec.Code)
	}
	for _, want := range []string{"refresh-e2e", "manual-all", "/feed-refresh/refresh-e2e"} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("list page missing %q", want)
		}
	}

	// Detail page.
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/feed-refresh/refresh-e2e", nil)
	req.SetPathValue("id", "refresh-e2e")
	a.handleRefreshRun(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /feed-refresh/{id} status = %d, want 200", rec.Code)
	}
	for _, want := range []string{"Hacker News", "Broken", "failed to fetch feed: 503", "fetch failed", "RAWFEEDBYTES", "application/rss"} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("detail page missing %q", want)
		}
	}

	// Unknown run → 404.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/feed-refresh/nope", nil)
	req.SetPathValue("id", "nope")
	a.handleRefreshRun(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET unknown run status = %d, want 404", rec.Code)
	}
}
