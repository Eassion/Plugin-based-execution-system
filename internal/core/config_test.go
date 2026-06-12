package core_test

import (
	"os"
	"path/filepath"
	"testing"

	"plugin-execution-system/internal/core"
)

func TestLoadConfigReadsPluginEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plugins.json")
	content := []byte(`{"plugins":[{"name":"uppercase","enabled":true},{"name":"timestamp","enabled":false}]}`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	config, err := core.LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}

	if len(config.Plugins) != 2 {
		t.Fatalf("expected 2 plugins, got %d", len(config.Plugins))
	}
	if config.Plugins[0].Name != "uppercase" || !config.Plugins[0].Enabled {
		t.Fatalf("unexpected first plugin: %#v", config.Plugins[0])
	}
	if config.Plugins[1].Name != "timestamp" || config.Plugins[1].Enabled {
		t.Fatalf("unexpected second plugin: %#v", config.Plugins[1])
	}
}
