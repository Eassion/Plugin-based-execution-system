package core_test

import (
	"context"
	"testing"

	"plugin-execution-system/internal/core"
)

func TestManagerBuildsExecutionPlanInDependencyOrder(t *testing.T) {
	manager := core.NewManager()
	_ = manager.Register(core.NewPlugin("wordcount", "1.0.0", passThrough))
	_ = manager.Register(core.NewPlugin("uppercase", "1.0.0", passThrough))
	_ = manager.Register(core.NewPlugin("timestamp", "1.0.0", passThrough))

	plan, err := manager.BuildExecutionPlan([]core.PluginConfig{
		{Name: "wordcount", Enabled: true, DependsOn: []string{"uppercase"}},
		{Name: "timestamp", Enabled: true, DependsOn: []string{"wordcount"}},
		{Name: "uppercase", Enabled: true},
	})
	if err != nil {
		t.Fatalf("BuildExecutionPlan returned error: %v", err)
	}

	names := []string{plan[0].Plugin.Name(), plan[1].Plugin.Name(), plan[2].Plugin.Name()}
	want := []string{"uppercase", "wordcount", "timestamp"}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("expected order %v, got %v", want, names)
		}
	}
}

func TestManagerRejectsDisabledDependency(t *testing.T) {
	manager := core.NewManager()
	_ = manager.Register(core.NewPlugin("wordcount", "1.0.0", passThrough))
	_ = manager.Register(core.NewPlugin("uppercase", "1.0.0", passThrough))

	_, err := manager.BuildExecutionPlan([]core.PluginConfig{
		{Name: "wordcount", Enabled: true, DependsOn: []string{"uppercase"}},
		{Name: "uppercase", Enabled: false},
	})
	if err == nil {
		t.Fatal("expected disabled dependency error")
	}
}

func TestManagerRejectsCircularDependency(t *testing.T) {
	manager := core.NewManager()
	_ = manager.Register(core.NewPlugin("a", "1.0.0", passThrough))
	_ = manager.Register(core.NewPlugin("b", "1.0.0", passThrough))

	_, err := manager.BuildExecutionPlan([]core.PluginConfig{
		{Name: "a", Enabled: true, DependsOn: []string{"b"}},
		{Name: "b", Enabled: true, DependsOn: []string{"a"}},
	})
	if err == nil {
		t.Fatal("expected circular dependency error")
	}
}

func passThrough(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
	return data, nil
}
