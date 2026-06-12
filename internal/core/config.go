package core

import (
	"encoding/json"
	"os"
)

type Config struct {
	Plugins []PluginConfig `json:"plugins"`
}

type PluginConfig struct {
	Name      string   `json:"name"`
	Enabled   bool     `json:"enabled"`
	DependsOn []string `json:"depends_on,omitempty"`
	TimeoutMS int      `json:"timeout_ms,omitempty"`
}

func LoadConfig(path string) (Config, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	var config Config
	if err := json.Unmarshal(content, &config); err != nil {
		return Config{}, err
	}

	return config, nil
}
