package store

import (
	"testing"

	"github.com/ma111e/downlink/pkg/models"
)

func TestGetOrCreateCategoryCreatesWithDefaults(t *testing.T) {
	s := newTestStore(t)
	cat, err := s.GetOrCreateCategory("security")
	if err != nil {
		t.Fatalf("GetOrCreateCategory() error = %v", err)
	}
	if cat.Name != "security" {
		t.Fatalf("Name = %q, want security", cat.Name)
	}
	if cat.Color != "#808080" || cat.Icon != "category" {
		t.Fatalf("defaults = (%q,%q), want (#808080, category)", cat.Color, cat.Icon)
	}
}

func TestGetOrCreateCategoryReturnsExistingWithoutOverwriting(t *testing.T) {
	s := newTestStore(t)
	// Seed via Create (SaveCategory uses gorm.Save, which needs an existing row).
	if err := s.db.Create(&models.Category{Name: "ai", Color: "#ff0000", Icon: "robot"}).Error; err != nil {
		t.Fatalf("seed category error = %v", err)
	}

	cat, err := s.GetOrCreateCategory("ai")
	if err != nil {
		t.Fatalf("GetOrCreateCategory() error = %v", err)
	}
	// Must return the stored row, not clobber it with the defaults.
	if cat.Color != "#ff0000" || cat.Icon != "robot" {
		t.Fatalf("category = (%q,%q), want existing (#ff0000, robot) preserved", cat.Color, cat.Icon)
	}

	// And it must not have created a duplicate.
	cats, err := s.GetCategories()
	if err != nil {
		t.Fatalf("GetCategories() error = %v", err)
	}
	if len(cats) != 1 {
		t.Fatalf("category count = %d, want 1 (no duplicate created)", len(cats))
	}
}
