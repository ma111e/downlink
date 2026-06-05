package models

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Selectors defines CSS selectors for content extraction
type Selectors struct {
	Article   string `json:"article,omitempty" yaml:"article,omitempty"`     // Selector to find the article content
	Cutoff    string `json:"cutoff,omitempty" yaml:"cutoff,omitempty"`       // Selector to mark where to cutoff the article
	Blacklist string `json:"blacklist,omitempty" yaml:"blacklist,omitempty"` // Elements to exclude from the article
}

// FeedsFile is the top-level structure of feeds.yml
type FeedsFile struct {
	DefaultSelectors *Selectors   `yaml:"default_selectors,omitempty"`
	Feeds            []FeedConfig `yaml:"feeds"`
}

// FeedConfig represents the configuration for a feed
type FeedConfig struct {
	URL       string            `json:"url" yaml:"url"`
	Title     string            `json:"title,omitempty" yaml:"title,omitempty"`
	Note      string            `json:"note,omitempty" yaml:"note,omitempty"`
	Type      string            `json:"type" yaml:"type"`
	Enabled   bool              `json:"enabled" yaml:"enabled"`
	Scraper   map[string]any    `json:"scraper,omitempty" yaml:"scraper,omitempty"`
	Scraping  string            `json:"scraping,omitempty" yaml:"scraping,omitempty"` // "dynamic", "full_browser", or "" (static)
	Selectors *Selectors        `json:"selectors,omitempty" yaml:"selectors,omitempty"`
	Headers   map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"` // custom HTTP headers applied to all requests for this feed
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
	TotalFetched     int
	Stored           int
	Skipped          int
	Errors           []string
	StoredArticleIDs []string // IDs of articles successfully stored in this fetch
}

// FeedDiagnosis is the structured result of inspecting a single feed's raw HTTP
// response. It captures what actually came back over the wire so the two common
// failure modes — an unrecognizable body ("Failed to detect feed type") and raw
// non-UTF-8 bytes ("invalid utf-8 syntax") — can be diagnosed without re-running
// the server at trace log level.
type FeedDiagnosis struct {
	URL             string `json:"url"`
	FinalURL        string `json:"final_url"`         // after redirects
	HTTPStatus      int    `json:"http_status"`       // 0 when the request never completed
	ContentType     string `json:"content_type"`      // raw Content-Type header
	ContentLength   int    `json:"content_length"`    // bytes actually read
	FeedTypeGuess   string `json:"feed_type_guess"`   // rss | atom | json-feed | html | empty | unknown
	DeclaredCharset string `json:"declared_charset"`  // from XML prolog or Content-Type, when present
	ItemCount       int    `json:"item_count"`        // parsed items when the feed is valid
	ParseError      string `json:"parse_error"`       // gofeed parse error, empty when valid
	InvalidUTF8At   *int   `json:"invalid_utf8_at"`   // byte offset of first invalid UTF-8 byte, nil when valid
	Verdict         string `json:"verdict"`           // one-line human summary
	BodySnippet     string `json:"body_snippet"`      // first printable bytes of the body
	HexDump         string `json:"hex_dump"`          // bytes around InvalidUTF8At, when relevant
	RawBodyPath     string `json:"raw_body_path"`     // on-disk path to the saved raw body
	FetchDurationMs int64  `json:"fetch_duration_ms"` // wall time of the fetch
}
