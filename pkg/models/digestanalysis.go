package models

// DigestAnalysis links an ArticleAnalysis to a Digest with duplicate-group metadata.
type DigestAnalysis struct {
	DigestId            string           `gorm:"primaryKey;index" json:"digest_id"`
	AnalysisId          string           `gorm:"primaryKey" json:"analysis_id"`
	ArticleId           string           `gorm:"index" json:"article_id"`
	DuplicateGroup      string           `json:"duplicate_group,omitempty"`
	IsMostComprehensive bool             `gorm:"default:false" json:"is_most_comprehensive"`
	Analysis            *ArticleAnalysis `gorm:"foreignKey:AnalysisId;references:Id" json:"analysis,omitempty"`
}

func (DigestAnalysis) TableName() string {
	return "digest_analyses"
}
