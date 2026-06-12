package core

import "sync"

type Runtime struct {
	mu       sync.RWMutex
	pipeline Pipeline
	config   Config
}

func NewRuntime(manager *Manager, config Config) (*Runtime, error) {
	runtime := &Runtime{}
	if err := runtime.Reload(manager, config); err != nil {
		return nil, err
	}
	return runtime, nil
}

func (r *Runtime) Reload(manager *Manager, config Config) error {
	enabled, err := manager.BuildExecutionPlan(config.Plugins)
	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.config = config
	r.pipeline = NewPipeline(enabled, config.Plugins)
	return nil
}

func (r *Runtime) Run(data map[string]interface{}) ExecutionResult {
	r.mu.RLock()
	pipeline := r.pipeline
	r.mu.RUnlock()

	return pipeline.Run(data)
}

func (r *Runtime) Config() Config {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.config
}
