package models

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Selectors defines CSS selectors for content extraction
type Selectors struct {
	Article   string `json:"article,omitempty" yaml:"article,omitempty"`   // Selector to find the article content
	Cutoff    string `json:"cutoff,omitempty" yaml:"cutoff,omitempty"`    // Selector to mark where to cutoff the article
	Blacklist string `json:"blacklist,omitempty" yaml:"blacklist,omitempty"` // Elements to exclude from the article
}

// FeedsFile is the top-level structure of feeds.yml
type FeedsFile struct {
	DefaultSelectors *Selectors   `yaml:"default_selectors,omitempty"`
	Feeds            []FeedConfig `yaml:"feeds"`
}

// FeedConfig represents the configuration for a feed
type FeedConfig struct {
	URL       string         `json:"url" yaml:"url"`
	Title     string         `json:"title,omitempty" yaml:"title,omitempty"`
	Type      string         `json:"type" yaml:"type"`
	Enabled   bool           `json:"enabled" yaml:"enabled"`
	Scraper   map[string]any `json:"scraper,omitempty" yaml:"scraper,omitempty"`
	Scraping  string         `json:"scraping,omitempty" yaml:"scraping,omitempty"` // "dynamic", "full_browser", or "" (static)
	Selectors *Selectors     `json:"selectors,omitempty" yaml:"selectors,omitempty"`
}

// Feed represents a feed with its metadata
type Feed struct {
	Id        string    `gorm:"primaryKey" json:"id"`
	URL       string    `gorm:"index" json:"url"`
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	LastFetch time.Time `json:"last_fetch"`
	// Scraper   map[string]any `gorm:"-" json:"scraper,omitempty"` // In-memory representation
	Scraper  datatypes.JSONMap `json:"scraper,omitempty"` // In-memory representation
	Enabled  *bool             `gorm:"default:true" json:"enabled"`
	GroupId  *string           `gorm:"default:'default'" json:"group_id"`
	Articles []Article         `gorm:"foreignKey:FeedId" json:"-"` // One-to-many relationship with articles
}

// TableName specifies the table name for Feed
func (Feed) TableName() string {
	return "feeds"
}

// BeforeCreate ensures Id is set and properly converts Params before creating a record
func (f *Feed) BeforeCreate(_ *gorm.DB) error {
	if f.Id == "" {
		// You may want to implement a UUId generation here if not already handled elsewhere
		return gorm.ErrInvalidField
	}

	return nil
}

// FeedItem represents a generic feed item returned by a scraper
type FeedItem struct {
	Id          string
	Title       string
	Content     string
	Link        string
	PublishedAt time.Time
	Tags        []string
	Category    string
	HeroImage   string // New field for hero image URL
}

// FeedResult represents the result of fetching a feed
type FeedResult struct {
	Feed        Feed
	Items       []FeedItem
	Error       error
	FetchResult FetchResult
}

// FetchResult holds statistics from a single feed fetch operation
type FetchResult struct {
	TotalFetched int
	Stored       int
	Skipped      int
	Errors       []string
}
