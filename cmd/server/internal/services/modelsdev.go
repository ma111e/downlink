package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"downlink/cmd/server/internal/config"
	"downlink/pkg/models"
)

const (
	modelsDevURL = "https://models.dev/api.json"
	modelsDevTTL = 24 * time.Hour
)

// modelsDevProviderKey maps a downlink provider type to its models.dev top-level key.
var modelsDevProviderKey = map[string]string{
	"openai":    "openai",
	"anthropic": "anthropic",
	"mistral":   "mistral",
}

type modelsDevRegistry map[string]modelsDevProvider

type modelsDevProvider struct {
	Models map[string]modelsDevModel `json:"models"`
}

type modelsDevModel struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

// modelsDevCacheFile is the on-disk cache shape.
type modelsDevCacheFile struct {
	FetchedAt time.Time         `json:"fetched_at"`
	Registry  modelsDevRegistry `json:"registry"`
}

var (
	modelsDevMu       sync.Mutex
	modelsDevMemo     modelsDevRegistry
	modelsDevMemoTime time.Time
)

func modelsDevCachePath() string {
	return filepath.Join(filepath.Dir(config.ConfigPath), "models_dev_cache.json")
}

// fetchModelsDevRegistry returns the models.dev registry, using an in-memory memo
// and an on-disk cache (both governed by modelsDevTTL) before falling back to an
// HTTP fetch. On a fetch failure with no fresh cache it returns the error.
func fetchModelsDevRegistry() (modelsDevRegistry, error) {
	modelsDevMu.Lock()
	defer modelsDevMu.Unlock()

	now := time.Now()

	if modelsDevMemo != nil && now.Sub(modelsDevMemoTime) < modelsDevTTL {
		return modelsDevMemo, nil
	}

	cachePath := modelsDevCachePath()
	if data, err := os.ReadFile(cachePath); err == nil {
		var cached modelsDevCacheFile
		if json.Unmarshal(data, &cached) == nil && cached.Registry != nil &&
			now.Sub(cached.FetchedAt) < modelsDevTTL {
			modelsDevMemo = cached.Registry
			modelsDevMemoTime = cached.FetchedAt
			return modelsDevMemo, nil
		}
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(modelsDevURL)
	if err != nil {
		return nil, fmt.Errorf("fetch models.dev: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("models.dev returned status %d", resp.StatusCode)
	}

	var registry modelsDevRegistry
	if err := json.NewDecoder(resp.Body).Decode(&registry); err != nil {
		return nil, fmt.Errorf("decode models.dev: %w", err)
	}

	cacheData, err := json.Marshal(modelsDevCacheFile{FetchedAt: now, Registry: registry})
	if err == nil {
		tmpPath := cachePath + ".tmp"
		if os.WriteFile(tmpPath, cacheData, 0600) == nil {
			if os.Rename(tmpPath, cachePath) != nil {
				_ = os.Remove(tmpPath)
			}
		}
	}

	modelsDevMemo = registry
	modelsDevMemoTime = now
	return registry, nil
}

// getModelsDevModelList returns the model list for a cloud provider type from models.dev.
func getModelsDevModelList(providerType string) ([]models.ModelInfo, error) {
	key, ok := modelsDevProviderKey[providerType]
	if !ok {
		return nil, fmt.Errorf("no models.dev mapping for provider type %q", providerType)
	}

	registry, err := fetchModelsDevRegistry()
	if err != nil {
		return nil, err
	}

	provider, ok := registry[key]
	if !ok {
		return nil, fmt.Errorf("provider %q not found in models.dev registry", key)
	}

	infos := make([]models.ModelInfo, 0, len(provider.Models))
	for _, m := range provider.Models {
		infos = append(infos, models.ModelInfo{
			Id:           m.Id,
			Name:         m.Id,
			DisplayName:  m.Name,
			ProviderType: providerType,
		})
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Id < infos[j].Id })

	return infos, nil
}
