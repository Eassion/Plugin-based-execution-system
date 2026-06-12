package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"plugin-execution-system/internal/core"
	"plugin-execution-system/internal/plugins"
)

func main() {
	configPath := flag.String("config", "plugins.json", "plugin config file")
	input := flag.String("input", "hello plugin system", "input text")
	flag.Parse()

	config, err := core.LoadConfig(*configPath)
	if err != nil {
		exitf("load config: %v", err)
	}

	manager := core.NewManager()
	plugins.RegisterDefaults(manager)

	enabled, err := manager.BuildExecutionPlan(config.Plugins)
	if err != nil {
		exitf("load plugins: %v", err)
	}

	pipeline := core.NewPipeline(enabled, config.Plugins)
	result := pipeline.Run(map[string]interface{}{"input": *input})

	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		exitf("encode result: %v", err)
	}

	fmt.Println(string(output))
}

func exitf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
