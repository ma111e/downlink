package store

import (
	"errors"
	"fmt"

	"downlink/pkg/models"
	"gorm.io/gorm"
)

func (s *GormStore) GetCategories() ([]models.Category, error) {
	var categories []models.Category

	if err := s.db.Find(&categories).Error; err != nil {
		return nil, fmt.Errorf("failed to get categories: %w", err)
	}

	return categories, nil
}

func (s *GormStore) SaveCategory(category models.Category) error {
	if err := s.db.Save(&category).Error; err != nil {
		return fmt.Errorf("failed to save category: %w", err)
	}

	return nil
}

// GetOrCreateCategory returns a category with the given name, creating it if it doesn't exist.
func (s *GormStore) GetOrCreateCategory(name string) (*models.Category, error) {
	var cat models.Category
	err := s.db.Where("name = ?", name).First(&cat).Error
	if err == nil {
		return &cat, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to look up category: %w", err)
	}
	// Category doesn't exist, create it
	cat = models.Category{
		Name:  name,
		Color: "#808080",
		Icon:  "category",
	}
	if err := s.db.Create(&cat).Error; err != nil {
		return nil, fmt.Errorf("failed to create category: %w", err)
	}
	return &cat, nil
}
