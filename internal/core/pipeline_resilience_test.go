package core_test

import (
	"context"
	"strings"
	"testing"

	"plugin-execution-system/internal/core"
)

func TestPipelineRecordsSkippedDisabledPlugins(t *testing.T) {
	pipeline := core.NewPipeline([]core.ConfiguredPlugin{}, []core.PluginConfig{
		{Name: "timestamp", Enabled: false},
	})

	result := pipeline.Run(map[string]interface{}{"input": "hello"})

	if len(result.PluginResults) != 1 {
		t.Fatalf("expected 1 plugin result, got %d", len(result.PluginResults))
	}
	if result.PluginResults[0].Status != core.StatusSkipped {
		t.Fatalf("expected skipped status, got %s", result.PluginResults[0].Status)
	}
}

func TestPipelineRecordsDuration(t *testing.T) {
	pipeline := core.NewPipeline([]core.ConfiguredPlugin{
		{Plugin: core.NewPlugin("fast", "1.0.0", passThrough), Config: core.PluginConfig{Name: "fast", Enabled: true}},
	}, nil)

	result := pipeline.Run(map[string]interface{}{"input": "hello"})

	if result.PluginResults[0].DurationMS < 0 {
		t.Fatalf("expected non-negative duration, got %d", result.PluginResults[0].DurationMS)
	}
}

func TestPipelineFailsPluginOnTimeoutAndContinues(t *testing.T) {
	pipeline := core.NewPipeline([]core.ConfiguredPlugin{
		{
			Plugin: core.NewPlugin("slow", "1.0.0", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
				<-ctx.Done()
				data["slow"] = true
				return data, ctx.Err()
			}),
			Config: core.PluginConfig{Name: "slow", Enabled: true, TimeoutMS: 5},
		},
		{
			Plugin: core.NewPlugin("next", "1.0.0", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
				data["next"] = true
				return data, nil
			}),
			Config: core.PluginConfig{Name: "next", Enabled: true},
		},
	}, nil)

	result := pipeline.Run(map[string]interface{}{"input": "hello"})

	if result.Data["slow"] == true {
		t.Fatalf("timed out plugin should not update pipeline data: %#v", result.Data)
	}
	if result.Data["next"] != true {
		t.Fatalf("next plugin should still run: %#v", result.Data)
	}
	if result.PluginResults[0].Status != core.StatusFailed {
		t.Fatalf("expected timeout plugin to fail, got %s", result.PluginResults[0].Status)
	}
}

func TestPipelineRecoversFromPanicAndContinues(t *testing.T) {
	pipeline := core.NewPipeline([]core.ConfiguredPlugin{
		{
			Plugin: core.NewPlugin("panic", "1.0.0", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
				panic("broken plugin")
			}),
			Config: core.PluginConfig{Name: "panic", Enabled: true},
		},
		{
			Plugin: core.NewPlugin("next", "1.0.0", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
				data["next"] = true
				return data, nil
			}),
			Config: core.PluginConfig{Name: "next", Enabled: true},
		},
	}, nil)

	result := pipeline.Run(map[string]interface{}{"input": "hello"})

	if result.Data["next"] != true {
		t.Fatalf("next plugin should run after panic: %#v", result.Data)
	}
	if result.PluginResults[0].Status != core.StatusFailed {
		t.Fatalf("expected panic plugin to fail, got %s", result.PluginResults[0].Status)
	}
}

func TestPipelineUsesEffectiveTimeoutInErrorMessage(t *testing.T) {
	pipeline := core.NewPipeline([]core.ConfiguredPlugin{
		{
			Plugin: core.NewPlugin("slow", "1.0.0", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
				<-ctx.Done()
				return data, ctx.Err()
			}),
			Config: core.PluginConfig{Name: "slow", Enabled: true, TimeoutMS: 5},
		},
	}, nil)

	result := pipeline.Run(map[string]interface{}{"input": "hello"})

	if !strings.Contains(result.PluginResults[0].Error, "5ms") {
		t.Fatalf("expected error to include effective timeout, got %q", result.PluginResults[0].Error)
	}
}

func TestPipelineClonesNestedData(t *testing.T) {
	originalTags := []interface{}{"a"}
	pipeline := core.NewPipeline([]core.ConfiguredPlugin{
		{
			Plugin: core.NewPlugin("mutator", "1.0.0", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
				nested := data["nested"].(map[string]interface{})
				tags := nested["tags"].([]interface{})
				tags[0] = "changed"
				nested["value"] = "changed"
				return data, nil
			}),
			Config: core.PluginConfig{Name: "mutator", Enabled: true},
		},
	}, nil)

	input := map[string]interface{}{"input": "hello", "nested": map[string]interface{}{"value": "original", "tags": originalTags}}
	pipeline.Run(input)

	nested := input["nested"].(map[string]interface{})
	if nested["value"] != "original" || originalTags[0] != "a" {
		t.Fatalf("expected original nested data unchanged, got %#v", input)
	}
}

func TestPipelineRejectsTimeoutAboveMaximum(t *testing.T) {
	pipeline := core.NewPipeline([]core.ConfiguredPlugin{
		{
			Plugin: core.NewPlugin("slow", "1.0.0", passThrough),
			Config: core.PluginConfig{Name: "slow", Enabled: true, TimeoutMS: core.MaxPluginTimeoutMS + 1},
		},
	}, nil)

	result := pipeline.Run(map[string]interface{}{"input": "hello"})

	if result.PluginResults[0].Status != core.StatusFailed {
		t.Fatalf("expected invalid timeout to fail, got %s", result.PluginResults[0].Status)
	}
}

func TestNewPluginRejectsNilRunner(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected NewPlugin to panic for nil runner")
		}
	}()

	core.NewPlugin("nil", "1.0.0", nil)
}
