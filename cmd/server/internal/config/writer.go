package config

import (
	"downlink/pkg/models"
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// Mu protects Config and config.json from concurrent reads/writes.
// Readers take RLock; the save path takes Lock.
var Mu sync.RWMutex

// configFileLayout is the on-disk shape of config.json.
// Feeds are excluded — they live in feeds.yml via models.ServerConfig.Save.
type configFileLayout struct {
	DbPath        string                      `json:"db_path"`
	Providers     []models.ProviderConfig     `json:"providers"`
	Analysis      models.AnalysisConfig       `json:"analysis"`
	Notifications models.NotificationsConfig  `json:"notifications"`
}

// SaveConfig serialises the provider/analysis/notifications fields of cfg to
// ConfigPath atomically (write to tmp, then rename) and replaces the
// in-memory Config pointer. Feeds are excluded (they live in feeds.yml).
// The file is written with mode 0600 because it may contain OAuth tokens.
func SaveConfig(cfg *models.ServerConfig) error {
	if cfg == nil {
		return fmt.Errorf("SaveConfig: nil config")
	}

	layout := configFileLayout{
		DbPath:        cfg.DbPath,
		Providers:     cfg.Providers,
		Analysis:      cfg.Analysis,
		Notifications: cfg.Notifications,
	}
	data, err := json.MarshalIndent(layout, "", "  ")
	if err != nil {
		return fmt.Errorf("SaveConfig: marshal: %w", err)
	}

	tmpPath := ConfigPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("SaveConfig: write tmp: %w", err)
	}
	if err := os.Rename(tmpPath, ConfigPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("SaveConfig: rename: %w", err)
	}

	Mu.Lock()
	Config = cfg
	Mu.Unlock()

	return nil
}
