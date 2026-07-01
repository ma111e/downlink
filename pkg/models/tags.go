package models

// Tag represents an article tag
type Tag struct {
	Id       string    `gorm:"primaryKey" json:"id"`
	Name     string    `gorm:"uniqueIndex" json:"name"`
	Color    string    `json:"color"`
	Articles []Article `gorm:"many2many:article_tags;" json:"-"` // Many-to-many relationship with articles
}

// TableName specifies the table name for Tag
func (Tag) TableName() string {
	return "tags"
}
