package models

// FeedGroup represents a group of feeds
type FeedGroup struct {
	Id        string `gorm:"primaryKey" json:"id"`
	Name      string `gorm:"uniqueIndex" json:"name"`
	Icon      string `json:"icon"`
	SortOrder *int   `gorm:"default:0" json:"sort_order"`
	Feeds     []Feed `gorm:"foreignKey:GroupId" json:"-"` // One-to-many relationship with feeds
}

// TableName specifies the table name for FeedGroup
func (FeedGroup) TableName() string {
	return "feed_groups"
}
