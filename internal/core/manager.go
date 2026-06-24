package core

import (
	"fmt"
	"sync"
)

type Manager struct {
	mu      sync.RWMutex
	plugins map[string]Plugin
}

func NewManager() *Manager {
	return &Manager{plugins: make(map[string]Plugin)}
}

func (m *Manager) Register(plugin Plugin) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.plugins[plugin.Name()]; exists {
		return fmt.Errorf("plugin %q is already registered", plugin.Name())
	}
	m.plugins[plugin.Name()] = plugin
	return nil
}

func (m *Manager) BuildExecutionPlan(configs []PluginConfig) ([]ConfiguredPlugin, error) {
	m.mu.RLock()
	plugins := make(map[string]Plugin, len(m.plugins))
	for name, plugin := range m.plugins {
		plugins[name] = plugin
	}
	m.mu.RUnlock()

	enabledConfigs := make(map[string]PluginConfig)
	seenConfigs := make(map[string]bool)
	for _, config := range configs {
		if seenConfigs[config.Name] {
			return nil, fmt.Errorf("duplicate plugin config %q", config.Name)
		}
		seenConfigs[config.Name] = true
		if !config.Enabled {
			continue
		}
		if _, ok := plugins[config.Name]; !ok {
			return nil, fmt.Errorf("enabled plugin %q is not registered", config.Name)
		}
		enabledConfigs[config.Name] = config
	}

	for _, config := range enabledConfigs {
		for _, dependency := range config.DependsOn {
			if _, ok := enabledConfigs[dependency]; !ok {
				return nil, fmt.Errorf("plugin %q depends on disabled or missing plugin %q", config.Name, dependency)
			}
		}
	}

	visited := make(map[string]bool)
	visiting := make(map[string]bool)
	ordered := make([]ConfiguredPlugin, 0, len(enabledConfigs))

	var visit func(name string) error
	visit = func(name string) error {
		if visited[name] {
			return nil
		}
		if visiting[name] {
			return fmt.Errorf("circular dependency detected at plugin %q", name)
		}

		visiting[name] = true
		config := enabledConfigs[name]
		for _, dependency := range config.DependsOn {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		visiting[name] = false
		visited[name] = true

		ordered = append(ordered, ConfiguredPlugin{
			Plugin: plugins[name],
			Config: config,
		})
		return nil
	}

	for _, config := range configs {
		if !config.Enabled {
			continue
		}
		if err := visit(config.Name); err != nil {
			return nil, err
		}
	}

	return ordered, nil
}

func (m *Manager) All() []PluginInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	infos := make([]PluginInfo, 0, len(m.plugins))
	for _, plugin := range m.plugins {
		infos = append(infos, PluginInfo{
			Name:    plugin.Name(),
			Version: plugin.Version(),
		})
	}
	return infos
}

type PluginInfo struct {
	Name        string   `json:"name"`
	PluginID    string   `json:"plugin_id,omitempty"`
	Version     string   `json:"version"`
	DependsOn   []string `json:"depends_on,omitempty"`
	UDSPath     string   `json:"uds_path,omitempty"`
	Description string   `json:"description,omitempty"`
	Enabled     bool     `json:"enabled,omitempty"`
	Healthy     bool     `json:"healthy,omitempty"`
	Draining    bool     `json:"draining,omitempty"`
}

type ConfiguredPlugin struct {
	Plugin Plugin
	Config PluginConfig
}
