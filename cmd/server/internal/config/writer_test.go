package config

import (
	"os"
	"testing"

	"github.com/ma111e/downlink/pkg/models"
)

func TestSaveConfigNilReturnsError(t *testing.T) {
	if err := SaveConfig(nil); err == nil {
		t.Fatal("SaveConfig(nil) error = nil, want error")
	}
}

func TestSaveConfigWritesFileWith0600(t *testing.T) {
	path := withConfigState(t)
	cfg := &models.ServerConfig{DbPath: "/db", Providers: []models.ProviderConfig{{Name: "p"}}}

	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("file mode = %o, want 600 (may hold OAuth tokens)", perm)
	}
}

func TestSaveConfigRoundTripExcludesFeeds(t *testing.T) {
	withConfigState(t)
	cfg := &models.ServerConfig{
		DbPath:    "/db",
		Feeds:     []models.FeedConfig{{}}, // must NOT be persisted to config.json
		Providers: []models.ProviderConfig{{Name: "codex-sub", ProviderType: "openai-codex", ModelName: "codex-mini", Enabled: true}},
		Analysis:  models.AnalysisConfig{Provider: "codex-sub", Persona: "be terse"},
		Notifications: models.NotificationsConfig{
			Discord: models.DiscordNotificationConfig{Enabled: true, WebhookURL: "https://hook"},
		},
	}

	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	loaded, err := LoadConfigFromFile(ConfigPath)
	if err != nil {
		t.Fatalf("LoadConfigFromFile() error = %v", err)
	}

	if len(loaded.Feeds) != 0 {
		t.Errorf("loaded.Feeds = %+v, want empty (feeds live in feeds.yml)", loaded.Feeds)
	}
	if len(loaded.Providers) != 1 || loaded.Providers[0].Name != "codex-sub" || loaded.Providers[0].ModelName != "codex-mini" {
		t.Errorf("loaded.Providers = %+v, want round-tripped codex-sub/codex-mini", loaded.Providers)
	}
	if loaded.Analysis.Persona != "be terse" {
		t.Errorf("loaded.Analysis.Persona = %q, want be terse", loaded.Analysis.Persona)
	}
	if !loaded.Notifications.Discord.Enabled || loaded.Notifications.Discord.WebhookURL != "https://hook" {
		t.Errorf("loaded.Notifications.Discord = %+v, want enabled hook round-tripped", loaded.Notifications.Discord)
	}
}

func TestSaveConfigUpdatesInMemoryPointer(t *testing.T) {
	withConfigState(t)
	cfg := &models.ServerConfig{DbPath: "/db", Providers: []models.ProviderConfig{{Name: "p"}}}

	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	if Config != cfg {
		t.Fatalf("Config = %p, want the saved pointer %p", Config, cfg)
	}
}
