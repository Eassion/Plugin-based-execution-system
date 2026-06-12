package core

import "context"

type Plugin interface {
	Name() string
	Version() string
	Run(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error)
}

type Runner func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error)

type basicPlugin struct {
	name    string
	version string
	run     Runner
}

func NewPlugin(name string, version string, run Runner) Plugin {
	if run == nil {
		panic("plugin runner cannot be nil")
	}
	return basicPlugin{name: name, version: version, run: run}
}

func (p basicPlugin) Name() string {
	return p.name
}

func (p basicPlugin) Version() string {
	return p.version
}

func (p basicPlugin) Run(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
	return p.run(ctx, data)
}
