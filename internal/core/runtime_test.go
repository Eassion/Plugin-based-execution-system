package core_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"plugin-execution-system/internal/core"
)

func TestRuntimeReloadReplacesPipeline(t *testing.T) {
	manager := core.NewManager()
	_ = manager.Register(core.NewPlugin("uppercase", "1.0.0", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
		data["input"] = "HELLO"
		return data, nil
	}))

	runtime, err := core.NewRuntime(manager, core.Config{
		Plugins: []core.PluginConfig{{Name: "uppercase", Enabled: true}},
	})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}

	before := runtime.Run(map[string]interface{}{"input": "hello"})
	if before.Data["input"] != "HELLO" {
		t.Fatalf("expected enabled plugin to update input, got %#v", before.Data)
	}

	err = runtime.Reload(manager, core.Config{
		Plugins: []core.PluginConfig{{Name: "uppercase", Enabled: false}},
	})
	if err != nil {
		t.Fatalf("Reload returned error: %v", err)
	}

	after := runtime.Run(map[string]interface{}{"input": "hello"})
	if after.Data["input"] != "hello" {
		t.Fatalf("expected disabled plugin to be unloaded, got %#v", after.Data)
	}
	if len(after.PluginResults) != 1 || after.PluginResults[0].Status != core.StatusSkipped {
		t.Fatalf("expected disabled plugin to be reported as skipped, got %#v", after.PluginResults)
	}
}

func TestWatchConfigReloadsChangedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plugins.json")
	if err := os.WriteFile(path, []byte(`{"plugins":[]}`), 0644); err != nil {
		t.Fatalf("write initial config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reloaded := make(chan core.Config, 1)
	errs := make(chan error, 1)
	go func() {
		errs <- core.WatchConfig(ctx, path, func(config core.Config) error {
			reloaded <- config
			return nil
		})
	}()

	if err := os.WriteFile(path, []byte(`{"plugins":[{"name":"uppercase","enabled":true}]}`), 0644); err != nil {
		t.Fatalf("write changed config: %v", err)
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case config := <-reloaded:
			if len(config.Plugins) == 1 && config.Plugins[0].Name == "uppercase" && config.Plugins[0].Enabled {
				goto reloaded
			}
		case err := <-errs:
			t.Fatalf("WatchConfig returned early: %v", err)
		case <-deadline:
			t.Fatal("timed out waiting for config reload")
		}
	}

reloaded:

	cancel()
	select {
	case err := <-errs:
		if err != nil {
			t.Fatalf("WatchConfig returned error after cancel: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for watcher shutdown")
	}
}
