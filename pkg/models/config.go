package models

import (
	"encoding/json"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ServerConfig represents application configuration
type ServerConfig struct {
	Feeds            []FeedConfig        `json:"feeds"`
	DbPath           string              `json:"db_path"`
	Providers        []ProviderConfig    `json:"providers"`
	Analysis         AnalysisConfig      `json:"analysis"`
	Notifications    NotificationsConfig `json:"notifications"`
	DefaultSelectors *Selectors          `json:"-" yaml:"-"` // Loaded from feeds.yml, not config.json
	SolimenAddr      string              `json:"solimen_addr"`
}

// NotificationsConfig holds notification platform configurations
type NotificationsConfig struct {
	Discord     DiscordNotificationConfig     `json:"discord"`
	GitHubPages GitHubPagesNotificationConfig `json:"github_pages"`
}

// DiscordNotificationConfig holds Discord-specific notification settings
type DiscordNotificationConfig struct {
	Enabled    bool   `json:"enabled"`
	WebhookURL string `json:"webhook_url"`
}

// GitHubPagesNotificationConfig holds GitHub Pages publishing settings
type GitHubPagesNotificationConfig struct {
	Enabled           bool   `json:"enabled"`
	RepoURL           string `json:"repo_url"`            // e.g. https://github.com/user/user.github.io.git
	Branch            string `json:"branch"`              // default "main"
	ConfigurePages    bool   `json:"configure_pages"`     // configure GitHub Pages source to this branch
	Token             string `json:"token"`               // GitHub PAT; prefer env DOWNLINK_GH_PAGES_TOKEN
	OutputDir         string `json:"output_dir"`          // subdirectory inside repo (empty = repo root)
	BaseURL           string `json:"base_url"`            // public URL, e.g. https://user.github.io
	CommitAuthor      string `json:"commit_author"`       // default "downlink-bot"
	CommitEmail       string `json:"commit_email"`        // default "downlink-bot@users.noreply.github.com"
	CloneDir          string `json:"clone_dir"`           // local working clone; default: os.TempDir()/downlink-ghpages
	DiscordWebhookURL string `json:"discord_webhook_url"` // optional: notify this webhook when a page is published
}

func (sc *ServerConfig) Save(path string) error {
	// Marshal only the file-persisted fields, excluding feeds (stored in feeds.yml)
	configFile := struct {
		DbPath        string              `json:"db_path"`
		Providers     []ProviderConfig    `json:"providers"`
		Analysis      AnalysisConfig      `json:"analysis"`
		Notifications NotificationsConfig `json:"notifications"`
	}{
		DbPath:        sc.DbPath,
		Providers:     sc.Providers,
		Analysis:      sc.Analysis,
		Notifications: sc.Notifications,
	}
	data, err := json.MarshalIndent(configFile, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return err
	}

	feedsPath := filepath.Join(filepath.Dir(path), "feeds.yml")
	return sc.SaveFeeds(feedsPath)
}

func (sc *ServerConfig) SaveFeeds(path string) error {
	ff := FeedsFile{
		DefaultSelectors: sc.DefaultSelectors,
		Feeds:            sc.Feeds,
	}
	data, err := yaml.Marshal(ff)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
