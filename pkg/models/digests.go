package models

import (
	"time"
)

// ProviderConfig represents configuration for a specific LLM provider used for digest generation
type ProviderConfig struct {
	Name           string   `json:"name"`                      // User-defined name for this provider configuration (required)
	ProviderType   string   `json:"provider_type"`
	ModelName      string   `json:"model_name"`
	Enabled        bool     `json:"enabled"`
	BaseURL        string   `json:"base_url,omitempty"`        // Used for Ollama and other local providers
	Temperature    *float64 `json:"temperature,omitempty"`     // Using pointer type for zero values (GORM best practice)
	MaxRetries     *int     `json:"max_retries,omitempty"`     // Using pointer type for zero values (GORM best practice)
	TimeoutMinutes *int     `json:"timeout_minutes,omitempty"` // Using pointer type for zero values (GORM best practice)
	APIKey         string   `json:"api_key,omitempty"`         // Per-provider API key; overrides the global key when set
}

// AnalysisConfig represents the configuration for the analysis features
type AnalysisConfig struct {
	Provider   string            `json:"provider,omitempty"`      // Name of the configured provider to use for analysis
	Persona    string            `json:"persona,omitempty"`       // Additional prompt prefix to customize the AI instructions
	WorkerPool *WorkerPoolConfig `json:"worker_pool,omitempty"`   // Configuration for the analysis worker pool
}

// WorkerPoolConfig contains the configuration for the worker pool
type WorkerPoolConfig struct {
	MaxWorkers *int `json:"max_workers,omitempty"` // Maximum number of concurrent workers (default: 3)
}

// Digest represents a generated digest of articles
type Digest struct {
	Id                  string                 `gorm:"primaryKey" json:"id"`
	CreatedAt           time.Time              `gorm:"index" json:"created_at"`
	Title               string                 `gorm:"type:text" json:"title,omitempty"`
	ArticleCount        *int                   `gorm:"default:0" json:"article_count"`
	TimeWindow          time.Duration          `json:"time_window"`
	RawGroupingResponse string                 `gorm:"type:text" json:"raw_grouping_response,omitempty"`
	DigestSummary       string                 `gorm:"type:text" json:"digest_summary,omitempty"`
	ProviderResults     []DigestProviderResult `gorm:"-" json:"provider_results"`          // Handled through separate table
	DigestAnalyses      []DigestAnalysis       `gorm:"-" json:"digest_analyses,omitempty"` // Handled through separate table
	Articles            []Article              `gorm:"many2many:digest_articles;" json:"-"`
}

// TableName specifies the table name for Digest
func (Digest) TableName() string {
	return "digests"
}

// Legacy DigestProviderResult has been moved to digestproviderresults.go

// DigestArticle represents an article included in a digest
type DigestArticle struct {
	DigestId  string `json:"digest_id"`
	ArticleId string `json:"article_id"`
}
