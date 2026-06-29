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

// ApplyFeeds should only report a feed as Updated when its config actually
// changed; re-applying an unchanged file must be a no-op.
func TestApplyFeeds_OnlyReportsRealChanges(t *testing.T) {
	db, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	m := NewFeedManager(db)
	m.RegisterScraper("rss", &stubScraper{})

	base := func() models.FeedConfig {
		return models.FeedConfig{
			URL:     "https://example.com/rss",
			Title:   "Example",
			Enabled: true,
			Topics:  []string{"tech", "news"},
			Scraper: models.ScraperConfig{
				Type:     "rss",
				Scraping: "none",
				Selectors: &models.Selectors{
					Article: ".content",
				},
			},
		}
	}

	// First apply: feed is created.
	res, err := m.ApplyFeeds([]models.FeedConfig{base()}, nil, false)
	if err != nil {
		t.Fatalf("ApplyFeeds (create): %v", err)
	}
	if len(res.Created) != 1 || len(res.Updated) != 0 || len(res.Disabled) != 0 {
		t.Fatalf("create: got %+v, want 1 created", res)
	}

	// Re-apply identical config: nothing to do.
	res, err = m.ApplyFeeds([]models.FeedConfig{base()}, nil, false)
	if err != nil {
		t.Fatalf("ApplyFeeds (resync): %v", err)
	}
	if len(res.Created)+len(res.Updated)+len(res.Disabled) != 0 {
		t.Fatalf("resync: got %+v, want nothing to do", res)
	}

	// Topic order should not matter.
	reordered := base()
	reordered.Topics = []string{"news", "tech"}
	res, err = m.ApplyFeeds([]models.FeedConfig{reordered}, nil, false)
	if err != nil {
		t.Fatalf("ApplyFeeds (reordered topics): %v", err)
	}
	if len(res.Updated) != 0 {
		t.Fatalf("reordered topics: got %+v, want no update", res)
	}

	// Each meaningful field change should mark exactly that feed Updated.
	mutators := map[string]func(*models.FeedConfig){
		"title":    func(c *models.FeedConfig) { c.Title = "Renamed" },
		"selector": func(c *models.FeedConfig) { c.Scraper.Selectors.Article = ".other" },
		"topics":   func(c *models.FeedConfig) { c.Topics = []string{"tech"} },
		"scraping": func(c *models.FeedConfig) { c.Scraper.Scraping = "dynamic" },
	}
	for name, mutate := range mutators {
		t.Run(name, func(t *testing.T) {
			cfg := base()
			mutate(&cfg)
			// dry-run so the stored feed stays at the baseline for the next case.
			res, err := m.ApplyFeeds([]models.FeedConfig{cfg}, nil, true)
			if err != nil {
				t.Fatalf("ApplyFeeds (%s): %v", name, err)
			}
			if len(res.Updated) != 1 || len(res.Created) != 0 {
				t.Fatalf("%s: got %+v, want 1 updated", name, res)
			}
		})
	}

	// Disabling the feed should report it as Disabled, not Updated.
	disabled := base()
	disabled.Enabled = false
	res, err = m.ApplyFeeds([]models.FeedConfig{disabled}, nil, false)
	if err != nil {
		t.Fatalf("ApplyFeeds (disable): %v", err)
	}
	if len(res.Disabled) != 1 || len(res.Updated) != 0 {
		t.Fatalf("disable: got %+v, want 1 disabled", res)
	}
}
