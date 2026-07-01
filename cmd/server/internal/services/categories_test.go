package services

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ma111e/downlink/cmd/server/internal/store"
	"github.com/ma111e/downlink/pkg/protos"
)

// withTempStore points the global store.Db at a fresh temp-file DB for the test.
func withTempStore(t *testing.T) *store.GormStore {
	t.Helper()
	s, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	orig := store.Db
	store.Db = s
	t.Cleanup(func() {
		store.Db = orig
		_ = s.Close()
	})
	return s
}

func TestGetOrCreateCategoryReturnsExisting(t *testing.T) {
	s := withTempStore(t)
	// Seed a category through the store (its GetOrCreateCategory uses Create).
	if _, err := s.GetOrCreateCategory("ai"); err != nil {
		t.Fatalf("seed error = %v", err)
	}

	srv := NewCategoriesServer()
	resp, err := srv.GetOrCreateCategory(context.Background(), &protos.GetOrCreateCategoryRequest{Name: "ai"})
	if err != nil {
		t.Fatalf("GetOrCreateCategory() error = %v", err)
	}
	if resp.Category == nil || resp.Category.Name != "ai" {
		t.Fatalf("category = %+v, want existing ai", resp.Category)
	}

	// A second lookup must not create a duplicate row.
	cats, err := s.GetCategories()
	if err != nil {
		t.Fatalf("GetCategories() error = %v", err)
	}
	n := 0
	for _, c := range cats {
		if c.Name == "ai" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("ai category count = %d, want 1 (no duplicate)", n)
	}
}

// TestGetOrCreateCategoryCreatePathIsBroken documents that the create branch of
// the CategoriesServer RPC currently FAILS for a brand-new category: it persists
// via store.SaveCategory (gorm.Save), but models.Category's primary key is not
// recognized (tag `uniqueIndex,primaryKey` is malformed), so Save issues an
// UPDATE with no WHERE clause and errors. Categories are created elsewhere via
// store.GetOrCreateCategory (which uses Create) instead. If SaveCategory/the
// model tag is fixed, this test should flip to expect a successful create.
func TestGetOrCreateCategoryCreatePathIsBroken(t *testing.T) {
	withTempStore(t)
	srv := NewCategoriesServer()
	_, err := srv.GetOrCreateCategory(context.Background(), &protos.GetOrCreateCategoryRequest{Name: "brand-new"})
	if err == nil {
		t.Fatal("GetOrCreateCategory(new) error = nil; the create path may have been fixed — update this test to assert success")
	}
}
