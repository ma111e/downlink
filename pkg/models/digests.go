package models

import (
	"time"
)

// CodexCredential holds OAuth state for a single ChatGPT/Codex account.
type CodexCredential struct {
	Id               string     `json:"id"`                          // short random ID
	Label            string     `json:"label"`                       // email or fallback
	Priority         int        `json:"priority"`                    // lower = preferred
	AccessToken      string     `json:"access_token"`
	RefreshToken     string     `json:"refresh_token"`
	LastRefresh      time.Time  `json:"last_refresh"`
	AuthMode         string     `json:"auth_mode"`                   // "chatgpt"
	Source           string     `json:"source"`                      // "manual:device_code"
	LastStatus       string     `json:"last_status,omitempty"`       // "ok" | "auth_failed" | "rate_limited"
	LastStatusAt     *time.Time `json:"last_status_at,omitempty"`
	LastErrorReason  string     `json:"last_error_reason,omitempty"`
	LastErrorResetAt *time.Time `json:"last_error_reset_at,omitempty"` // when rate_limited expires
}

// ProviderConfig represents configuration for a specific LLM provider used for digest generation
type ProviderConfig struct {
	Name           string            `json:"name"`                      // User-defined name for this provider configuration (required)
	ProviderType   string            `json:"provider_type"`
	ModelName      string            `json:"model_name"`
	Enabled        bool              `json:"enabled"`
	BaseURL        string            `json:"base_url,omitempty"`        // Used for Ollama and other local providers
	Temperature    *float64          `json:"temperature,omitempty"`     // Using pointer type for zero values (GORM best practice)
	MaxRetries     *int              `json:"max_retries,omitempty"`     // Using pointer type for zero values (GORM best practice)
	TimeoutMinutes *int              `json:"timeout_minutes,omitempty"` // Using pointer type for zero values (GORM best practice)
	APIKey         string            `json:"api_key,omitempty"`         // Per-provider API key; overrides the global key when set
	Credentials    []CodexCredential `json:"credentials,omitempty"`     // openai-codex OAuth credential pool
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
