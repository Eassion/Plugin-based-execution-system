package core_test

import (
	"context"
	"errors"
	"testing"

	"plugin-execution-system/internal/core"
)

func TestManagerFiltersEnabledPluginsInConfigOrder(t *testing.T) {
	manager := core.NewManager()
	_ = manager.Register(core.NewPlugin("uppercase", "1.0.0", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
		data["input"] = "HELLO"
		return data, nil
	}))
	_ = manager.Register(core.NewPlugin("disabled", "1.0.0", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
		data["disabled"] = true
		return data, nil
	}))

	plugins, err := manager.BuildExecutionPlan([]core.PluginConfig{
		{Name: "uppercase", Enabled: true},
		{Name: "disabled", Enabled: false},
	})
	if err != nil {
		t.Fatalf("Enabled returned error: %v", err)
	}

	if len(plugins) != 1 {
		t.Fatalf("expected 1 enabled plugin, got %d", len(plugins))
	}
	if plugins[0].Plugin.Name() != "uppercase" {
		t.Fatalf("expected uppercase plugin, got %q", plugins[0].Plugin.Name())
	}
}

func TestManagerReturnsErrorForMissingEnabledPlugin(t *testing.T) {
	manager := core.NewManager()

	_, err := manager.BuildExecutionPlan([]core.PluginConfig{{Name: "missing", Enabled: true}})
	if err == nil {
		t.Fatal("expected missing plugin error")
	}
}

func TestManagerRejectsDuplicatePluginConfigNames(t *testing.T) {
	manager := core.NewManager()
	_ = manager.Register(core.NewPlugin("uppercase", "1.0.0", passThrough))

	_, err := manager.BuildExecutionPlan([]core.PluginConfig{
		{Name: "uppercase", Enabled: true},
		{Name: "uppercase", Enabled: true, DependsOn: []string{"other"}},
	})
	if err == nil {
		t.Fatal("expected duplicate plugin config error")
	}
}

func TestManagerRejectsDuplicateRegisteredPluginNames(t *testing.T) {
	manager := core.NewManager()
	if err := manager.Register(core.NewPlugin("uppercase", "1.0.0", passThrough)); err != nil {
		t.Fatalf("first register returned error: %v", err)
	}

	err := manager.Register(core.NewPlugin("uppercase", "2.0.0", passThrough))
	if err == nil {
		t.Fatal("expected duplicate registered plugin error")
	}
}

func TestPipelineContinuesAfterPluginError(t *testing.T) {
	pipeline := core.NewPipeline([]core.ConfiguredPlugin{
		{Plugin: core.NewPlugin("bad", "1.0.0", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
			return data, errors.New("boom")
		}), Config: core.PluginConfig{Name: "bad", Enabled: true}},
		{Plugin: core.NewPlugin("good", "1.0.0", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
			data["processed"] = true
			return data, nil
		}), Config: core.PluginConfig{Name: "good", Enabled: true}},
	}, nil)

	result := pipeline.Run(map[string]interface{}{"input": "hello"})

	if result.Data["processed"] != true {
		t.Fatalf("expected later plugin to run after error, got %#v", result.Data)
	}
	if len(result.PluginResults) != 2 {
		t.Fatalf("expected 2 plugin results, got %d", len(result.PluginResults))
	}
	if result.PluginResults[0].Status != core.StatusFailed {
		t.Fatalf("expected first plugin to fail, got %s", result.PluginResults[0].Status)
	}
	if result.PluginResults[1].Status != core.StatusSuccess {
		t.Fatalf("expected second plugin to succeed, got %s", result.PluginResults[1].Status)
	}
}
