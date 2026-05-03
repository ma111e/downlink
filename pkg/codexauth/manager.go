package codexauth

import (
	"downlink/pkg/models"
	"sync"
)

// ConfigSaver is the function the manager calls to atomically persist config.
type ConfigSaver func(cfg *models.ServerConfig) error

// ConfigGetter returns the current in-memory config.
type ConfigGetter func() *models.ServerConfig

// Manager holds one Pool per openai-codex provider config entry and wires
// pool persistence back to config.json via the ConfigSaver.
type Manager struct {
	mu        sync.Mutex
	pools     map[string]*Pool // keyed by provider config Name
	getConfig ConfigGetter
	saveConfig ConfigSaver
}

// NewManager creates a Manager and initialises pools from the current config.
func NewManager(get ConfigGetter, save ConfigSaver) *Manager {
	m := &Manager{
		pools:      make(map[string]*Pool),
		getConfig:  get,
		saveConfig: save,
	}
	m.Reload()
	return m
}

// Reload rebuilds pools from the current config. Call after config hot-reload.
func (m *Manager) Reload() {
	cfg := m.getConfig()
	if cfg == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range cfg.Providers {
		if p.ProviderType != "openai-codex" {
			continue
		}
		name := p.Name
		if pool, ok := m.pools[name]; ok {
			pool.UpdateCredentials(p.Credentials)
		} else {
			m.pools[name] = NewPool(p.Credentials, m.makePersist(name))
		}
	}
}

// Pool returns the Pool for the named provider config entry.
func (m *Manager) Pool(providerName string) (*Pool, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.pools[providerName]
	return p, ok
}

// EnsurePool creates an empty pool for providerName if one doesn't exist yet.
func (m *Manager) EnsurePool(providerName string) *Pool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.pools[providerName]; ok {
		return p
	}
	p := NewPool(nil, m.makePersist(providerName))
	m.pools[providerName] = p
	return p
}

// makePersist returns a PersistFn that writes updated credentials back to the
// named provider entry in config.json.
func (m *Manager) makePersist(providerName string) PersistFn {
	return func(creds []models.CodexCredential) error {
		cfg := m.getConfig()
		if cfg == nil {
			return nil
		}
		for i := range cfg.Providers {
			if cfg.Providers[i].Name == providerName {
				cfg.Providers[i].Credentials = creds
				return m.saveConfig(cfg)
			}
		}
		return nil
	}
}
