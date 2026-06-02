package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// DigestProviderResult represents a provider's result for a digest
type DigestProviderResult struct {
	Id                     string    `gorm:"primaryKey" json:"id"`
	DigestId               string    `gorm:"index" json:"digest_id"`
	ProviderType           string    `json:"provider_type"`
	ModelName              string    `json:"model_name"`
	BriefOverview          string    `gorm:"type:text" json:"brief_overview"`
	StandardSynthesis      string    `gorm:"type:text" json:"standard_synthesis"`
	ComprehensiveSynthesis string    `gorm:"type:text" json:"comprehensive_synthesis"`
	ProcessingTime         float64   `json:"processing_time"`
	Error                  string    `json:"error"`
	CreatedAt              time.Time `gorm:"index" json:"created_at"`
	Digest                 *Digest   `gorm:"foreignKey:DigestId" json:"-"`
}

// TableName specifies the table name for DigestProviderResult
func (DigestProviderResult) TableName() string {
	return "digest_provider_results"
}

// BeforeCreate ensures Id is set before creating a record
func (d *DigestProviderResult) BeforeCreate(tx *gorm.DB) error {
	if d.Id == "" {
		d.Id = uuid.New().String()
	}

	return nil
}
