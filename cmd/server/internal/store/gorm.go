package store

import (
	"downlink/cmd/server/internal/config"
	"downlink/pkg/models"
	"fmt"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// GormStore implements the Store interface using GORM
type GormStore struct {
	db *gorm.DB
}

var (
	Db *GormStore
)

func Init() error {
	var err error
	// Initialize the database with GORM
	Db, err = New(config.Config.DbPath)
	if err != nil {
		return err
	}
	return nil
}

// New creates a new GormStore instance
func New(path string) (*GormStore, error) {
	// Configure GORM to use SQLite
	storeConfig := &gorm.Config{
		Logger:                                   logger.Default.LogMode(logger.Warn),
		DisableForeignKeyConstraintWhenMigrating: true,
	}

	// Connect to the database
	db, err := gorm.Open(sqlite.Open(path), storeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable foreign keys for SQLite
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// Performance optimizations for SQLite
	if err := db.Exec("PRAGMA journal_mode = WAL").Error; err != nil {
		return nil, fmt.Errorf("failed to set journal mode: %w", err)
	}

	if err := db.Exec("PRAGMA synchronous = NORMAL").Error; err != nil {
		return nil, fmt.Errorf("failed to set synchronous mode: %w", err)
	}

	store := &GormStore{db: db}

	// Auto-migrate the schemas
	if err := store.initSchema(); err != nil {
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return store, nil
}

// Close closes the database connection by getting the underlying SQL DB instance
func (s *GormStore) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return fmt.Errorf("failed to get SQL DB instance: %w", err)
	}
	return sqlDB.Close()
}

// initSchema initializes the database schema using GORM's automigration
func (s *GormStore) initSchema() error {
	// List all allModels that need to be migrated
	allModels := []interface{}{
		&models.Category{},
		&models.Tag{},
		&models.Feed{},
		&models.Article{},
		&models.RelatedArticle{},
		&models.Digest{},
		&models.DigestProviderResult{},
		&models.ArticleAnalysis{},
		&models.DigestAnalysis{},
		&models.FeedGroup{},
		// Add any new allModels here
	}

	// Auto-migrate all allModels
	for _, model := range allModels {
		if err := s.db.AutoMigrate(model); err != nil {
			return fmt.Errorf("failed to migrate %T: %w", model, err)
		}
	}

	// Insert default feed group if it doesn't exist
	var count int64

	if err := s.db.Table("feed_groups").Where("id = ?", "default").Count(&count).Error; err != nil {
		return fmt.Errorf("failed to check for default feed group: %w", err)
	}

	if count == 0 {
		// Create a map with the default values
		defaultGroup := map[string]interface{}{
			"id":         "default",
			"name":       "Default",
			"sort_order": 0,
		}

		if err := s.db.Table("feed_groups").Create(defaultGroup).Error; err != nil {
			return fmt.Errorf("failed to create default feed group: %w", err)
		}
	}

	return nil
}
