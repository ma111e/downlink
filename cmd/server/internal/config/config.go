package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"downlink/pkg/models"

	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

var (
	Config     *models.ServerConfig
	ConfigPath = "config.json"
)

func Init() error {
	var err error
	Config, err = LoadConfigFromFile(ConfigPath)
	if err != nil {
		return err
	}

	return nil
}

// ReloadConfig reloads the configuration from file.
// On failure the existing Config is preserved so callers always have a valid pointer.
func ReloadConfig() error {
	newConfig, err := LoadConfigFromFile(ConfigPath)
	if err != nil {
		return err
	}
	Config = newConfig
	return nil
}

// LoadConfigFromFile loads configuration from a JSON file and feeds from feeds.yml
func LoadConfigFromFile(path string) (*models.ServerConfig, error) {
	var config *models.ServerConfig

	file, err := os.Open(path)
	if err != nil {
		return config, fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return config, fmt.Errorf("failed to read config file: %w", err)
	}

	if err := json.Unmarshal(data, &config); err != nil {
		return config, fmt.Errorf("failed to parse config file: %w", err)
	}

	for i, p := range config.Providers {
		if p.Name == "" {
			return nil, fmt.Errorf("provider at index %d is missing required field \"name\"", i)
		}
	}

	feedsPath := filepath.Join(filepath.Dir(path), "feeds.yml")
	feedsFile, err := loadFeedsFromFile(feedsPath)
	if os.IsNotExist(err) {
		log.WithField("path", feedsPath).Warn("feeds.yml not found; starting with no configured feeds")
	} else if err != nil {
		return config, fmt.Errorf("failed to load feeds config: %w", err)
	}
	if feedsFile != nil {
		config.Feeds = feedsFile.Feeds
		config.DefaultSelectors = feedsFile.DefaultSelectors
	}

	return config, nil
}

func loadFeedsFromFile(path string) (*models.FeedsFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var ff models.FeedsFile
	if err := yaml.Unmarshal(data, &ff); err != nil {
		return nil, fmt.Errorf("failed to parse feeds file: %w", err)
	}

	return &ff, nil
}
