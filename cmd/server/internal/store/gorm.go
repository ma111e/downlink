package store

import (
	"fmt"
	"github.com/ma111e/downlink/cmd/server/internal/config"
	"github.com/ma111e/downlink/pkg/models"
	golog "log"
	"os"
	"path/filepath"
	"time"

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
	gormLogger := logger.New(
		golog.New(os.Stderr, "\r\n", golog.LstdFlags),
		logger.Config{
			SlowThreshold:             200 * time.Millisecond,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		},
	)
	storeConfig := &gorm.Config{
		Logger:                                   gormLogger,
		DisableForeignKeyConstraintWhenMigrating: true,
	}

	// Resolve to an absolute path so the database is independent of the
	// process working directory (a relative "./downlink.db" would otherwise
	// silently become a different file when launched from another cwd).
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve database path %q: %w", path, err)
	}

	// Connect to the database
	db, err := gorm.Open(sqlite.Open(absPath), storeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// SQLite allows a single writer and each connection to an anonymous/":memory:"
	// DSN gets its own private database. Pin the pool to one connection so every
	// query shares the same database (the schema can never diverge across the
	// pool) and writes are serialized, avoiding "database is locked".
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get SQL DB instance: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)

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
		&models.GlossaryEntry{},
		&models.DigestGlossary{},
		&models.FeedGroup{},
		&models.LLMRun{},
		&models.LLMCall{},
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
