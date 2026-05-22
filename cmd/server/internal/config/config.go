package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"downlink/pkg/models"
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

// LoadConfigFromFile loads configuration from a JSON file.
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

	return config, nil
}
