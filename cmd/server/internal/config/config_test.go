package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ma111e/downlink/pkg/models"
)

// withConfigState points the package globals at an isolated temp file for the
// duration of a test and restores them afterwards, so tests don't leak the
// process-wide ConfigPath/Config into one another.
func withConfigState(t *testing.T) string {
	t.Helper()
	origPath, origConfig := ConfigPath, Config
	path := filepath.Join(t.TempDir(), "config.json")
	ConfigPath = path
	t.Cleanup(func() {
		ConfigPath = origPath
		Config = origConfig
	})
	return path
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func TestLoadConfigFromFileValid(t *testing.T) {
	path := withConfigState(t)
	writeFile(t, path, `{
		"db_path": "/data/downlink.db",
		"providers": [{"name": "codex-sub", "provider_type": "openai-codex", "model_name": "codex-mini", "enabled": true}],
		"analysis": {"provider": "codex-sub"}
	}`)

	cfg, err := LoadConfigFromFile(path)
	if err != nil {
		t.Fatalf("LoadConfigFromFile() error = %v", err)
	}
	if cfg.DbPath != "/data/downlink.db" {
		t.Errorf("DbPath = %q, want /data/downlink.db", cfg.DbPath)
	}
	if len(cfg.Providers) != 1 || cfg.Providers[0].Name != "codex-sub" {
		t.Errorf("Providers = %+v, want one named codex-sub", cfg.Providers)
	}
	if cfg.Analysis.Provider != "codex-sub" {
		t.Errorf("Analysis.Provider = %q, want codex-sub", cfg.Analysis.Provider)
	}
}

func TestLoadConfigFromFileMissingFile(t *testing.T) {
	cfg, err := LoadConfigFromFile(filepath.Join(t.TempDir(), "nope.json"))
	if err == nil {
		t.Fatal("LoadConfigFromFile() error = nil, want open error")
	}
	if !strings.Contains(err.Error(), "failed to open config file") {
		t.Errorf("error = %v, want it to mention open failure", err)
	}
	if cfg != nil {
		t.Errorf("config = %+v, want nil on error", cfg)
	}
}

func TestLoadConfigFromFileMalformedJSON(t *testing.T) {
	path := withConfigState(t)
	writeFile(t, path, `{"db_path": "x", not json`)

	_, err := LoadConfigFromFile(path)
	if err == nil || !strings.Contains(err.Error(), "failed to parse config file") {
		t.Fatalf("error = %v, want parse failure", err)
	}
}

func TestLoadConfigFromFileMissingDbPath(t *testing.T) {
	path := withConfigState(t)
	writeFile(t, path, `{"providers": []}`)

	cfg, err := LoadConfigFromFile(path)
	if err == nil || !strings.Contains(err.Error(), "db_path is required") {
		t.Fatalf("error = %v, want db_path required", err)
	}
	if cfg != nil {
		t.Errorf("config = %+v, want nil when db_path missing", cfg)
	}
}

func TestLoadConfigFromFileProviderMissingName(t *testing.T) {
	path := withConfigState(t)
	writeFile(t, path, `{"db_path": "x", "providers": [{"name": "ok"}, {"provider_type": "ollama"}]}`)

	_, err := LoadConfigFromFile(path)
	if err == nil || !strings.Contains(err.Error(), "index 1 is missing required field") {
		t.Fatalf("error = %v, want missing-name error pointing at index 1", err)
	}
}

func TestReloadConfigPreservesConfigOnFailure(t *testing.T) {
	withConfigState(t)
	sentinel := &models.ServerConfig{DbPath: "keep-me"}
	Config = sentinel
	ConfigPath = filepath.Join(t.TempDir(), "absent.json")

	if err := ReloadConfig(); err == nil {
		t.Fatal("ReloadConfig() error = nil, want failure on missing file")
	}
	if Config != sentinel {
		t.Fatalf("Config replaced on failure; got %+v, want the previous pointer", Config)
	}
}

func TestReloadConfigReplacesConfigOnSuccess(t *testing.T) {
	path := withConfigState(t)
	Config = &models.ServerConfig{DbPath: "old"}
	writeFile(t, path, `{"db_path": "new"}`)

	if err := ReloadConfig(); err != nil {
		t.Fatalf("ReloadConfig() error = %v", err)
	}
	if Config == nil || Config.DbPath != "new" {
		t.Fatalf("Config.DbPath = %v, want new", Config)
	}
}
