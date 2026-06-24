package manager

import (
	"path/filepath"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/ma111e/downlink/cmd/server/internal/scrapers"
	"github.com/ma111e/downlink/cmd/server/internal/store"
	"github.com/ma111e/downlink/pkg/models"

	"gorm.io/datatypes"
)

// stubScraper returns a fixed set of items and never re-scrapes.
type stubScraper struct {
	items []models.FeedItem
}

func (s *stubScraper) Fetch(url string, params map[string]any) ([]models.FeedItem, *scrapers.RawResponse, error) {
	return s.items, &scrapers.RawResponse{}, nil
}

func (s *stubScraper) ScrapeContent(url string, params map[string]any) (string, error) {
	return "", nil
}

// FetchFeed should keep an article whose content has invalid UTF-8, storing it
// with the invalid bytes stripped rather than dropping it as an error.
func TestFetchFeed_SanitizesInvalidUTF8Content(t *testing.T) {
	db, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	m := NewFeedManager(db)
	m.RegisterScraper("rss", &stubScraper{items: []models.FeedItem{{
		Id:          "item-1",
		Title:       "Café story",
		Link:        "https://example.com/a",
		Content:     "caf\xe9 bar", // stray Latin-1 0xe9, invalid UTF-8
		PublishedAt: time.Now(),
	}}})

	// scraping "none" keeps the feed's own content (no re-scrape).
	feed := models.Feed{
		Id:      "feed-1",
		URL:     "https://example.com/rss",
		Type:    "rss",
		Scraper: datatypes.JSONMap{"scraping": "none"},
	}

	result, err := m.FetchFeed(feed, nil, nil, false, false, 0)
	if err != nil {
		t.Fatalf("FetchFeed: %v", err)
	}

	if result.Stored != 1 {
		t.Fatalf("Stored = %d, want 1", result.Stored)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("Errors = %v, want none", result.Errors)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want 1 sanitize notice", result.Warnings)
	}
	if len(result.StoredArticleIDs) != 1 {
		t.Fatalf("StoredArticleIDs = %v, want 1 id", result.StoredArticleIDs)
	}

	art, err := db.GetArticle(result.StoredArticleIDs[0])
	if err != nil {
		t.Fatalf("GetArticle: %v", err)
	}
	if !utf8.ValidString(art.Content) {
		t.Fatalf("stored content is not valid UTF-8: %q", art.Content)
	}
	if art.Content != "caf bar" {
		t.Fatalf("Content = %q, want %q", art.Content, "caf bar")
	}
}
