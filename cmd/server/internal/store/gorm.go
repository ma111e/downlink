package store

import (
	"encoding/json"
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
		&models.FeedTopic{},
		&models.Article{},
		&models.RelatedArticle{},
		&models.Digest{},
		&models.DigestProviderResult{},
		&models.ArticleAnalysis{},
		&models.DigestAnalysis{},
		&models.GlossaryEntry{},
		&models.DigestGlossary{},
		&models.FeedGroup{},
		&models.Profile{},
		&models.LLMRun{},
		&models.LLMCall{},
		&models.FeedRefreshRun{},
		&models.FeedRefreshResult{},
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

	if err := s.seedDefaultProfile(); err != nil {
		return err
	}

	return nil
}

// seedDefaultProfile creates the "default" profile on first run and backfills
// profile_id on pre-existing analyses, digests, and LLM runs. It reproduces the
// historical single-tenant behavior: the default profile's editorial config is
// the current global AnalysisConfig (nil rubric/categories ⇒ package defaults),
// it owns every enabled feed, and all existing rows are attributed to it.
// Idempotent: a no-op once a "default" profile exists.
func (s *GormStore) seedDefaultProfile() error {
	var count int64
	if err := s.db.Table("profiles").Where("id = ?", "default").Count(&count).Error; err != nil {
		return fmt.Errorf("failed to check for default profile: %w", err)
	}
	if count > 0 {
		return nil
	}

	// The default profile stores an EMPTY editorial on purpose: an empty editorial
	// inherits the live global AnalysisConfig at resolve time (see
	// services.ResolveEditorial), so the default profile keeps tracking config.json
	// changes instead of freezing a snapshot taken at migration time.
	// config.Config can be nil when the store is opened outside the server (e.g.
	// tests via store.New); fall back to zero values for the presentation fields.
	layout := "default"
	outputSubdir := ""
	if config.Config != nil {
		if l := config.Config.Notifications.GitHubPages.Layout; l != "" {
			layout = l
		}
		outputSubdir = config.Config.Notifications.GitHubPages.OutputDir
	}

	editorialJSON, err := json.Marshal(&models.ProfileEditorial{})
	if err != nil {
		return fmt.Errorf("failed to marshal default profile editorial: %w", err)
	}

	defaultProfile := map[string]interface{}{
		"id":            "default",
		"name":          "Default",
		"layout":        layout,
		"output_subdir": outputSubdir,
		"enabled":       true,
		"sort_order":    0,
		"editorial":     string(editorialJSON),
	}
	if err := s.db.Table("profiles").Create(defaultProfile).Error; err != nil {
		return fmt.Errorf("failed to create default profile: %w", err)
	}

	// The default profile owns every currently-enabled feed.
	if err := s.db.Exec(
		"INSERT INTO profile_feeds (profile_id, feed_id) SELECT 'default', id FROM feeds WHERE enabled = 1",
	).Error; err != nil {
		return fmt.Errorf("failed to seed default profile feeds: %w", err)
	}

	// Backfill existing rows so every analysis/digest/run is attributed to a profile.
	for _, table := range []string{"article_analyses", "digests", "llm_runs"} {
		if err := s.db.Exec(
			fmt.Sprintf("UPDATE %s SET profile_id = 'default' WHERE profile_id = '' OR profile_id IS NULL", table),
		).Error; err != nil {
			return fmt.Errorf("failed to backfill profile_id on %s: %w", table, err)
		}
	}

	return nil
}
