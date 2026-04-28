package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ReferencedReport is an explicit third-party report, research item, advisory,
// whitepaper, or technical analysis linked from an article.
type ReferencedReport struct {
	Title     string `json:"title"`
	URL       string `json:"url"`
	Publisher string `json:"publisher"`
	Context   string `json:"context"`
}

// ArticleAnalysis represents an analysis result from an LLM provider for an article
type ArticleAnalysis struct {
	Id                     string             `gorm:"primaryKey" json:"id"`
	ArticleId              string             `gorm:"index" json:"article_id"`
	ProviderType           string             `json:"provider_type"`
	ModelName              string             `json:"model_name"`
	ImportanceScore        int                `json:"importance_score"`
	KeyPointsJson          string             `gorm:"column:key_points;type:text" json:"-"`
	InsightsJson           string             `gorm:"column:insights;type:text" json:"-"`
	ReferencedReportsJson  string             `gorm:"column:referenced_reports;type:text" json:"-"`
	KeyPoints              []string           `gorm:"-" json:"key_points"`
	Insights               []string           `gorm:"-" json:"insights"`
	ReferencedReports      []ReferencedReport `gorm:"-" json:"referenced_reports"`
	Tldr                   string             `gorm:"type:text" json:"tldr"`
	Justification          string             `gorm:"type:text" json:"justification"`
	BriefOverview          string             `gorm:"type:text" json:"brief_overview"`
	StandardSynthesis      string             `gorm:"type:text" json:"standard_synthesis"`
	ComprehensiveSynthesis string             `gorm:"type:text" json:"comprehensive_synthesis"`
	ThinkingProcess        string             `gorm:"type:text" json:"thinking_process,omitempty"`
	RawResponse            string             `gorm:"type:text" json:"raw_response"`
	CreatedAt              time.Time          `gorm:"index" json:"created_at"`
	Article                *Article           `gorm:"foreignKey:ArticleId;references:Id" json:"-"`
}

// TableName specifies the table name for ArticleAnalysis
func (ArticleAnalysis) TableName() string {
	return "article_analyses"
}

// BeforeCreate ensures Id is set before creating a record
func (a *ArticleAnalysis) BeforeCreate(tx *gorm.DB) error {
	if a.Id == "" {
		a.Id = uuid.New().String()
	}

	// Convert slices to JSON for database storage
	if len(a.KeyPoints) > 0 {
		keyPointsBytes, err := json.Marshal(a.KeyPoints)
		if err != nil {
			return err
		}
		a.KeyPointsJson = string(keyPointsBytes)
	}

	if len(a.Insights) > 0 {
		insightsBytes, err := json.Marshal(a.Insights)
		if err != nil {
			return err
		}
		a.InsightsJson = string(insightsBytes)
	}

	if len(a.ReferencedReports) > 0 {
		referencedReportsBytes, err := json.Marshal(a.ReferencedReports)
		if err != nil {
			return err
		}
		a.ReferencedReportsJson = string(referencedReportsBytes)
	}

	return nil
}

// AfterFind converts JSON strings back to slices after querying
func (a *ArticleAnalysis) AfterFind(tx *gorm.DB) error {
	// Convert JSON strings back to slices
	if a.KeyPointsJson != "" {
		if err := json.Unmarshal([]byte(a.KeyPointsJson), &a.KeyPoints); err != nil {
			return err
		}
	}

	if a.InsightsJson != "" {
		if err := json.Unmarshal([]byte(a.InsightsJson), &a.Insights); err != nil {
			return err
		}
	}

	if a.ReferencedReportsJson != "" {
		if err := json.Unmarshal([]byte(a.ReferencedReportsJson), &a.ReferencedReports); err != nil {
			return err
		}
	}

	return nil
}
