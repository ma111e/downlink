package models

import (
	"encoding/json"
	"time"

	"github.com/ma111e/downlink/pkg/scoring"

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
	Category  string `json:"category"`
	Primary   bool   `json:"primary"`
}

// GlossaryTerm is a single jargon term and its plain-language definition, produced
// by the glossary-mode analysis task to help newcomers familiarize themselves with
// the terminology used in an article. Type is the semantic category (see the
// GlossaryCategory* constants); Context is a one-sentence explanation of why the term
// matters in this specific article (per-occurrence, not global).
type GlossaryTerm struct {
	Term       string `json:"term"`
	Type       string `json:"type"`
	Definition string `json:"definition"`
	Context    string `json:"context"`
}

// ArticleAnalysis represents an analysis result from an LLM provider for an article
type ArticleAnalysis struct {
	Id                     string              `gorm:"primaryKey" json:"id"`
	ArticleId              string              `gorm:"index" json:"article_id"`
	ProviderType           string              `json:"provider_type"`
	ModelName              string              `json:"model_name"`
	ImportanceScore        int                 `json:"importance_score"`
	ScoreDimensionsJson    string              `gorm:"column:score_dimensions;type:text" json:"-"`
	ScoreDimensions        *scoring.Dimensions `gorm:"-" json:"score_dimensions,omitempty"`
	KeyPointsJson          string              `gorm:"column:key_points;type:text" json:"-"`
	InsightsJson           string              `gorm:"column:insights;type:text" json:"-"`
	ReferencedReportsJson  string              `gorm:"column:referenced_reports;type:text" json:"-"`
	KeyPoints              []string            `gorm:"-" json:"key_points"`
	Insights               []string            `gorm:"-" json:"insights"`
	ReferencedReports      []ReferencedReport  `gorm:"-" json:"referenced_reports"`
	Tldr                   string              `gorm:"type:text" json:"tldr"`
	PlainWords             string              `gorm:"type:text" json:"plain_words"`
	Justification          string              `gorm:"type:text" json:"justification"`
	BriefOverview          string              `gorm:"type:text" json:"brief_overview"`
	StandardSynthesis      string              `gorm:"type:text" json:"standard_synthesis"`
	ComprehensiveSynthesis string              `gorm:"type:text" json:"comprehensive_synthesis"`
	GlossaryTermsJson      string              `gorm:"column:glossary_terms;type:text" json:"-"`
	GlossaryTerms          []GlossaryTerm      `gorm:"-" json:"glossary_terms"`
	ThinkingProcess        string              `gorm:"type:text" json:"thinking_process,omitempty"`
	RawResponse            string              `gorm:"type:text" json:"raw_response"`
	CreatedAt              time.Time           `gorm:"index" json:"created_at"`
	Article                *Article            `gorm:"foreignKey:ArticleId;references:Id" json:"-"`
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

	if a.ScoreDimensions != nil {
		scoreDimensionsBytes, err := json.Marshal(a.ScoreDimensions)
		if err != nil {
			return err
		}
		a.ScoreDimensionsJson = string(scoreDimensionsBytes)
	}

	if len(a.GlossaryTerms) > 0 {
		glossaryTermsBytes, err := json.Marshal(a.GlossaryTerms)
		if err != nil {
			return err
		}
		a.GlossaryTermsJson = string(glossaryTermsBytes)
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

	if a.ScoreDimensionsJson != "" {
		var dims scoring.Dimensions
		if err := json.Unmarshal([]byte(a.ScoreDimensionsJson), &dims); err != nil {
			return err
		}
		a.ScoreDimensions = &dims
	}

	if a.GlossaryTermsJson != "" {
		if err := json.Unmarshal([]byte(a.GlossaryTermsJson), &a.GlossaryTerms); err != nil {
			return err
		}
	}

	return nil
}
