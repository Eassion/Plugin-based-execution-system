package core

import "fmt"

type DependencyGraph struct {
	deps map[string][]string
}

func NewDependencyGraph() *DependencyGraph {
	return &DependencyGraph{deps: make(map[string][]string)}
}

func (g *DependencyGraph) Snapshot() map[string][]string {
	snapshot := make(map[string][]string, len(g.deps))
	for id, deps := range g.deps {
		snapshot[id] = append([]string(nil), deps...)
	}
	return snapshot
}

func (g *DependencyGraph) Restore(snapshot map[string][]string) {
	g.deps = make(map[string][]string, len(snapshot))
	for id, deps := range snapshot {
		g.deps[id] = append([]string(nil), deps...)
	}
}

func (g *DependencyGraph) Put(pluginID string, deps []string) {
	g.deps[pluginID] = append([]string(nil), deps...)
}

func (g *DependencyGraph) ValidateAcyclic() error {
	visited := make(map[string]bool)
	visiting := make(map[string]bool)

	var visit func(string) error
	visit = func(pluginID string) error {
		if visited[pluginID] {
			return nil
		}
		if visiting[pluginID] {
			return fmt.Errorf("circular dependency detected at plugin %q", pluginID)
		}

		visiting[pluginID] = true
		for _, dependency := range g.deps[pluginID] {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		visiting[pluginID] = false
		visited[pluginID] = true
		return nil
	}

	for pluginID := range g.deps {
		if err := visit(pluginID); err != nil {
			return err
		}
	}
	return nil
}
