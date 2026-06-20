package models

import "time"

// LLMRun is one digest-generation run. Every LLM call made while generating a
// digest is correlated to a run via its Id (propagated through the call context
// by the gateway). DigestId/Title are filled in once the digest is created.
type LLMRun struct {
	Id         string     `gorm:"primaryKey" json:"id"`
	DigestId   string     `gorm:"index" json:"digest_id,omitempty"`
	Title      string     `json:"title,omitempty"`
	StartedAt  time.Time  `gorm:"index" json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

func (LLMRun) TableName() string { return "llm_runs" }

// LLMCall is one prompt/response that passed through the gateway during a run.
// Prompt and Response are stored gzip-compressed (the store layer owns the
// codec); token counts are zero with TokensKnown=false for backends that do not
// report usage (e.g. OAuth subscription providers).
type LLMCall struct {
	Id               string    `gorm:"primaryKey" json:"id"`
	RunId            string    `gorm:"index" json:"run_id"`
	Label            string    `gorm:"index" json:"label"`
	ProviderType     string    `json:"provider_type"`
	ModelName        string    `json:"model_name"`
	Prompt           []byte    `gorm:"type:blob" json:"-"` // gzip-compressed UTF-8
	Response         []byte    `gorm:"type:blob" json:"-"` // gzip-compressed UTF-8
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	TotalTokens      int       `json:"total_tokens"`
	TokensKnown      bool      `json:"tokens_known"`
	DurationMs       int64     `json:"duration_ms"`
	Err              string    `gorm:"column:error;type:text" json:"error,omitempty"`
	CreatedAt        time.Time `gorm:"index" json:"created_at"`
}

func (LLMCall) TableName() string { return "llm_calls" }
