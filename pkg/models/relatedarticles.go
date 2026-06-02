package models

// RelatedArticle represents a relationship between two articles
type RelatedArticle struct {
	ArticleId        string  `gorm:"primaryKey;column:article_id" json:"article_id"`
	RelatedArticleId string  `gorm:"primaryKey;column:related_article_id" json:"related_article_id"`
	RelationType     string  `gorm:"column:relation_type" json:"relation_type"` // e.g., "similar", "continuation", "response"
	SimilarityScore  float64 `gorm:"column:similarity_score" json:"similarity_score"`
}

// TableName specifies the table name for RelatedArticle
func (RelatedArticle) TableName() string {
	return "related_articles"
}
