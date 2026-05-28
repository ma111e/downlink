package scrapers

import (
	"downlink/pkg/models"
)

// Scraper defines the interface for feed scrapers
type Scraper interface {
	Fetch(url string) ([]models.FeedItem, error)
	ScrapeContent(url string, params map[string]any) (string, error)
}
