package manager

import (
	"downlink/cmd/server/internal/scrapers"
	"downlink/cmd/server/internal/store"
	"downlink/pkg/models"
	"fmt"
	"sync"

	log "github.com/sirupsen/logrus"
	"gorm.io/datatypes"
)

// FeedManager manages feeds and articles
type FeedManager struct {
	store             store.Store
	scrapers          map[string]scrapers.Scraper
	mu                sync.RWMutex
	anonymizedScraper *scrapers.AnonymizedScraper
	solimenAddr       string
}

var (
	Manager *FeedManager
)

func InitFeedManager(db store.Store) {
	Manager = NewFeedManager(db)
}

// NewFeedManager creates a new FeedManager instance
func NewFeedManager(db store.Store) *FeedManager {
	// Create a general-purpose anonymized scraper (no domain restriction)
	anonymizedScraper := scrapers.NewAnonymizedScraper("")

	return &FeedManager{
		store:             db,
		scrapers:          make(map[string]scrapers.Scraper),
		anonymizedScraper: anonymizedScraper,
	}
}

// RegisterScraper registers a scraper for a feed type
func (m *FeedManager) RegisterScraper(feedType string, scraper scrapers.Scraper) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.scrapers[feedType] = scraper
	log.WithField("type", feedType).Info("Registered scraper")
}

// SetSolimenAddr sets the solimen service address used for full_browser feeds.
func (m *FeedManager) SetSolimenAddr(addr string) {
	m.solimenAddr = addr
}

// GetScraper returns the scraper for a feed type
func (m *FeedManager) GetScraper(feedType string) (scrapers.Scraper, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	scraper, ok := m.scrapers[feedType]
	if !ok {
		return nil, fmt.Errorf("no scraper registered for feed type: %s", feedType)
	}

	return scraper, nil
}

// RegisterFeed registers a feed
func (m *FeedManager) RegisterFeed(config models.FeedConfig) error {
	// Check if scraper exists
	if _, err := m.GetScraper(config.Type); err != nil {
		return err
	}

	// Generate feed Id
	feedId := generateFeedId(config.URL)

	// Build the scraper params map, merging structured feed config fields in.
	scraperMap := make(datatypes.JSONMap)
	for k, v := range config.Scraper {
		scraperMap[k] = v
	}
	if config.Scraping != "" {
		scraperMap["scraping"] = config.Scraping
	}
	if config.Selectors != nil {
		sel := map[string]any{}
		if config.Selectors.Article != "" {
			sel["article"] = config.Selectors.Article
		}
		if config.Selectors.Cutoff != "" {
			sel["cutoff"] = config.Selectors.Cutoff
		}
		if config.Selectors.Blacklist != "" {
			sel["blacklist"] = config.Selectors.Blacklist
		}
		scraperMap["selectors"] = sel
	}

	// Create feed
	feed := models.Feed{
		Id:      feedId,
		URL:     config.URL,
		Type:    config.Type,
		Title:   config.Title,
		Scraper: scraperMap,
		Enabled: &config.Enabled,
	}

	// Store feed
	if err := m.store.StoreFeed(feed); err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"id":      feedId,
		"url":     config.URL,
		"type":    config.Type,
		"enabled": config.Enabled,
	}).Info("Feed registered")

	return nil
}
