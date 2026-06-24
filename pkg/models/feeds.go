package models

import (
	"fmt"
	"strings"
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

// FeedConfig represents the configuration for a feed. Everything scraping-related
// lives under the nested Scraper block; only identity fields stay at the top level.
type FeedConfig struct {
	URL     string        `json:"url" yaml:"url"`
	Title   string        `json:"title,omitempty" yaml:"title,omitempty"`
	Note    string        `json:"note,omitempty" yaml:"note,omitempty"`
	Enabled bool          `json:"enabled" yaml:"enabled"`
	Topics  []string      `json:"topics,omitempty" yaml:"topics,omitempty"` // labels profiles select feeds by
	Scraper ScraperConfig `json:"scraper" yaml:"scraper"`
}

// Validate enforces the hard requirements every feed config must meet before it
// can be registered: a non-empty title, and — for the html scraper — a
// date_selector.
func (fc FeedConfig) Validate() error {
	if strings.TrimSpace(fc.Title) == "" {
		return fmt.Errorf("feed %s: title is required", fc.URL)
	}
	if fc.Scraper.Type == "html" {
		ds, _ := fc.Scraper.Options["date_selector"].(string)
		if strings.TrimSpace(ds) == "" {
			return fmt.Errorf("feed %s: html scraper requires a 'date_selector'", fc.URL)
		}
	}
	return nil
}

// FeedTopic is one (feed, topic) membership row. Topics are operator-assigned
// labels; a profile selects feeds by topic (see ProfileSelection). Distinct from
// the LLM-generated article entity tags (see Tag).
type FeedTopic struct {
	FeedId string `gorm:"primaryKey" json:"feed_id"`
	Topic  string `gorm:"primaryKey;index" json:"topic"`
}

// TableName specifies the table name for FeedTopic.
func (FeedTopic) TableName() string { return "feed_topics" }

// ScraperConfig holds all scraping configuration for a feed: the scraper type,
// render mode, content selectors, custom headers, full_browser triggers, and any
// type-specific options. Type-specific keys (e.g. the html scraper's
// links_selector / url_filter) are captured by the inline Options map so adding a
// new scraper type needs no struct change here.
type ScraperConfig struct {
	Type      string            `json:"type" yaml:"type"`
	Scraping  string            `json:"scraping,omitempty" yaml:"scraping,omitempty"` // "dynamic", "full_browser", "none" (use feed content, no fetch), or "" (static)
	Selectors *Selectors        `json:"selectors,omitempty" yaml:"selectors,omitempty"`
	Headers   map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"` // custom HTTP headers applied to all requests for this feed
	Triggers  *HostTriggers     `json:"triggers,omitempty" yaml:"triggers,omitempty"`
	Options   map[string]any    `json:"-" yaml:",inline"` // type-specific flat keys (links_selector, url_filter, ...)
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
	// Topics are the feed's labels (profiles select feeds by topic). Stored in the
	// feed_topics table, not on this row; populated by ListFeeds for read paths.
	Topics []string `gorm:"-" json:"topics,omitempty"`
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
	Warnings         []string // non-fatal notices (e.g. content sanitized); the article was still stored
	StoredArticleIDs []string // IDs of articles successfully stored in this fetch

	// Raw feed response captured for the refresh monitor, so the exact bytes a
	// refresh saw (or failed to parse) stay inspectable. RawBody is nil when
	// nothing was fetched (e.g. a network-level error before any response).
	RawBody        []byte
	RawStatus      int
	RawContentType string
}

// SelectorCandidate is a ranked guess at the CSS selector wrapping an article body,
// produced by inspecting a scraped page. Mirrors scrapers.SelectorCandidate.
type SelectorCandidate struct {
	Selector    string  `json:"selector"`
	Chars       int     `json:"chars"`
	LinkDensity float64 `json:"link_density"`
	Snippet     string  `json:"snippet"`
}

// LinkListCandidate is a ranked guess at the repeating post-link structure on an
// HTML index page: a links_selector scoping the post anchors, plus the relative
// date_selector and url_filter inferred from the repeating block. Mirrors
// scrapers.LinkListCandidate.
type LinkListCandidate struct {
	LinksSelector string   `json:"links_selector"`
	Count         int      `json:"count"`                   // anchors the selector matched
	SampleHrefs   []string `json:"sample_hrefs"`            // a few resolved post URLs
	DateSelector  string   `json:"date_selector,omitempty"` // relative selector for the block's date
	DateSample    string   `json:"date_sample,omitempty"`   // raw text/attr of one matched date
	URLFilter     string   `json:"url_filter,omitempty"`    // shared path segment of the post URLs
}

// ArticleInspection is the result of scraping a single article URL in a given
// mode, used by the feed-config builder to inspect page HTML and test selectors.
type ArticleInspection struct {
	ModeUsed        string `json:"mode_used"`
	RawHTMLLen      int    `json:"raw_html_len"`
	HTML            string `json:"html"`             // page HTML (capped) for selector inspection
	Extracted       string `json:"extracted"`        // extracted markdown when selectors supplied
	ExtractedLen    int    `json:"extracted_len"`    // rune count of Extracted
	SelectorMatched bool   `json:"selector_matched"` // the article selector matched an element
	Error           string `json:"error"`
	DurationMs      int64  `json:"duration_ms"`
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

	// DiscoveredFeeds holds validated RSS/Atom/JSON feed URLs found on an HTML
	// page (via <link> autodiscovery, anchor keywords, or common-path probing).
	// Populated only when the fetched URL is itself an HTML page, so a caller can
	// redirect autoconfig at the real feed instead of the landing page.
	DiscoveredFeeds []string `json:"discovered_feeds,omitempty"`
}
