package models

import (
	"time"

	"gorm.io/gorm"
)

// Article represents an article from a feed
type Article struct {
	Id              string           `gorm:"primaryKey" json:"id"`
	FeedId          string           `gorm:"index" json:"feed_id"`
	Title           string           `json:"title"`
	Content         string           `gorm:"type:text" json:"content"`
	Link            string           `json:"link"`
	PublishedAt     time.Time        `gorm:"index" json:"published_at"`
	FetchedAt       time.Time        `json:"fetched_at"`
	Read            *bool            `gorm:"default:false" json:"read"`
	Tags            []Tag            `gorm:"many2many:article_tags;" json:"tags"`
	CategoryName    *string          `gorm:"index" json:"category_name,omitempty"`
	Category        *Category        `gorm:"foreignKey:CategoryName;references:Name" json:"category,omitempty"`
	HeroImage       string           `json:"hero_image,omitempty"`
	Bookmarked      *bool            `gorm:"default:false" json:"bookmarked"`
	RelatedArticles []RelatedArticle `gorm:"-" json:"related_articles,omitempty"` // Handled separately
	// LatestImportanceScore is populated only by ListArticles (via JOIN on the latest analysis).
	// Not persisted on the Article row.
	LatestImportanceScore *int32 `gorm:"-" json:"latest_importance_score,omitempty"`
}

type ArticleCounts struct {
	AllUnreadCount  int64            `json:"all_unread_count"`
	BookmarkedCount int64            `json:"bookmarked_count"`
	UnreadByFeed    map[string]int64 `json:"unread_by_feed"`
}

// TableName specifies the table name for Article
func (Article) TableName() string {
	return "articles"
}

// BeforeCreate ensures Id is set before creating a record
func (a *Article) BeforeCreate(_ *gorm.DB) error {
	if a.Id == "" {
		return gorm.ErrInvalidField
	}
	return nil
}

// ArticleUpdate represents fields that can be updated for an article
type ArticleUpdate struct {
	Read            *bool             `json:"read,omitempty"`
	TagIds          *[]string         `json:"tag_ids,omitempty"`
	CategoryName    *string           `json:"category_name,omitempty"`
	HeroImage       *string           `json:"hero_image,omitempty"`
	Bookmarked      *bool             `json:"bookmarked,omitempty"`
	RelatedArticles *[]RelatedArticle `json:"related_articles,omitempty"`
}

// ArticleFilter represents filtering options for listing articles
type ArticleFilter struct {
	UnreadOnly      bool       `json:"unread_only"`
	CategoryName    string     `json:"category_name"`
	TagId           string     `json:"tag_id"`
	BookmarkedOnly  bool       `json:"bookmarked_only"`
	RelatedToId     string     `json:"related_to_id"`
	StartDate       *time.Time `json:"start_date,omitempty"`
	EndDate         *time.Time `json:"end_date,omitempty"`
	FeedId          string     `json:"feed_id"`
	ExcludeDigested bool       `json:"exclude_digested,omitempty"`
	Offset          int        `json:"offset,omitempty"`
	Limit           int        `json:"limit,omitempty"`
	Query           string     `json:"query,omitempty"`
}
