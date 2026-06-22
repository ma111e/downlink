package manager

import (
	"encoding/json"
	"fmt"
	"github.com/ma111e/downlink/cmd/server/internal/scrapers"
	"github.com/ma111e/downlink/cmd/server/internal/store"
	"github.com/ma111e/downlink/pkg/models"
	"sync"

	log "github.com/sirupsen/logrus"
	"gorm.io/datatypes"
)

// FeedManager manages feeds and articles
type FeedManager struct {
	store       store.Store
	scrapers    map[string]scrapers.Scraper
	mu          sync.RWMutex
	solimenAddr string
}

var (
	Manager *FeedManager
)

func InitFeedManager(db store.Store) {
	Manager = NewFeedManager(db)
}

// NewFeedManager creates a new FeedManager instance
func NewFeedManager(db store.Store) *FeedManager {
	return &FeedManager{
		store:    db,
		scrapers: make(map[string]scrapers.Scraper),
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
	if _, err := m.GetScraper(config.Scraper.Type); err != nil {
		return err
	}

	// Generate feed Id
	feedId, err := generateFeedId(config.URL)
	if err != nil {
		return fmt.Errorf("invalid feed URL %s: %w", config.URL, err)
	}

	// Flatten the nested scraper config into the runtime params map. The fetch path
	// reads these keys flat off feed.Scraper, so the shape here must stay flat:
	// scraping / selectors / headers / triggers plus any type-specific options.
	scraperMap := make(datatypes.JSONMap)
	sc := config.Scraper
	for k, v := range sc.Options {
		scraperMap[k] = v
	}
	if sc.Scraping != "" {
		scraperMap["scraping"] = sc.Scraping
	}
	if sc.Selectors != nil {
		sel := map[string]any{}
		if sc.Selectors.Article != "" {
			sel["article"] = sc.Selectors.Article
		}
		if sc.Selectors.Cutoff != "" {
			sel["cutoff"] = sc.Selectors.Cutoff
		}
		if sc.Selectors.Blacklist != "" {
			sel["blacklist"] = sc.Selectors.Blacklist
		}
		scraperMap["selectors"] = sel
	}
	if len(sc.Headers) > 0 {
		headers := map[string]any{}
		for k, v := range sc.Headers {
			headers[k] = v
		}
		scraperMap["headers"] = headers
	}
	if sc.Triggers != nil {
		if b, merr := json.Marshal(sc.Triggers); merr == nil {
			var t map[string]any
			if json.Unmarshal(b, &t) == nil {
				scraperMap["triggers"] = t
			}
		}
	}

	// Create feed
	feed := models.Feed{
		Id:      feedId,
		URL:     config.URL,
		Type:    config.Scraper.Type,
		Title:   config.Title,
		Scraper: scraperMap,
		Enabled: &config.Enabled,
	}

	// Preserve runtime state of an existing feed: StoreFeed does a full upsert,
	// so without this the LastFetch timestamp and GroupId would be reset.
	if existing, err := m.store.GetFeed(feedId); err == nil {
		feed.LastFetch = existing.LastFetch
		feed.GroupId = existing.GroupId
	}

	// Store feed
	if err := m.store.StoreFeed(feed); err != nil {
		return err
	}

	// Replace the feed's topics (the labels profiles select feeds by).
	if err := m.store.SetFeedTopics(feedId, config.Topics); err != nil {
		return fmt.Errorf("failed to set feed topics for %s: %w", config.URL, err)
	}

	log.WithFields(log.Fields{
		"id":      feedId,
		"url":     config.URL,
		"type":    config.Scraper.Type,
		"enabled": config.Enabled,
		"topics":  config.Topics,
	}).Info("Feed registered")

	return nil
}

// AllTopics returns the distinct topics in use across all configured feeds.
func (m *FeedManager) AllTopics() ([]string, error) {
	return m.store.ListAllTopics()
}
