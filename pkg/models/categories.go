package models

// Category represents an article category
type Category struct {
	Name     string    `gorm:"uniqueIndex,primaryKey" json:"name"`
	Color    string    `json:"color"`                                            // For UI display
	Icon     string    `json:"icon"`                                             // For UI display
	Articles []Article `gorm:"foreignKey:CategoryName;references:Name" json:"-"` // One-to-many relationship with articles
}

// TableName specifies the table name for Category
func (Category) TableName() string {
	return "categories"
}
