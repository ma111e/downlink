package models

import "time"

// FeedRefreshRun is one feed-refresh cycle: a single feed refresh, a refresh of
// all feeds, or the startup refresh. Each enabled feed processed during the
// cycle has a child FeedRefreshResult correlated by the run Id.
type FeedRefreshRun struct {
	Id         string     `gorm:"primaryKey" json:"id"`
	Trigger    string     `json:"trigger"` // "startup" | "manual-all" | "manual-single"
	StartedAt  time.Time  `gorm:"index" json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

func (FeedRefreshRun) TableName() string { return "feed_refresh_runs" }

// FeedRefreshResult is the outcome of refreshing one feed during a run. Success
// reflects the top-level fetch (FetchError empty); item-level scrape/store
// failures are counted in ErrorCount and kept verbatim in ErrorLog. ErrorLog is
// stored gzip-compressed (the store layer owns the codec).
type FeedRefreshResult struct {
	Id           string    `gorm:"primaryKey" json:"id"`
	RunId        string    `gorm:"index" json:"run_id"`
	FeedId       string    `gorm:"index" json:"feed_id"`
	FeedTitle    string    `json:"feed_title"` // denormalized: the feed may be deleted later
	FeedURL      string    `json:"feed_url"`
	Success      bool      `json:"success"` // top-level fetch err == nil
	TotalFetched int       `json:"total_fetched"`
	Stored       int       `json:"stored"`
	Skipped      int       `json:"skipped"`
	ErrorCount   int       `json:"error_count"` // number of item-level errors
	FetchError   string    `gorm:"column:fetch_error;type:text" json:"fetch_error,omitempty"`
	ErrorLog     []byte    `gorm:"type:blob" json:"-"` // gzip of joined item-level errors
	RawBody      []byte    `gorm:"type:blob" json:"-"` // gzip of the raw fetched feed body
	RawStatus    int       `json:"raw_status,omitempty"`
	RawType      string    `json:"raw_type,omitempty"` // raw response Content-Type
	DurationMs   int64     `json:"duration_ms"`
	CreatedAt    time.Time `gorm:"index" json:"created_at"`
}

func (FeedRefreshResult) TableName() string { return "feed_refresh_results" }
