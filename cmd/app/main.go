package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"plugin-execution-system/internal/core"
	"plugin-execution-system/internal/plugins"
)

func main() {
	configPath := flag.String("config", "plugins.json", "plugin config file")
	input := flag.String("input", "hello plugin system", "input text")
	watch := flag.Bool("watch", false, "watch plugin config and reload execution plan")
	flag.Parse()

	manager := core.NewManager()
	plugins.RegisterDefaults(manager)

	if *watch {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()

		runtime := &core.Runtime{}
		if err := core.WatchConfig(ctx, *configPath, func(config core.Config) error {
			if err := runtime.Reload(manager, config); err != nil {
				return err
			}
			return printResult(runtime.Run(map[string]interface{}{"input": *input}))
		}); err != nil {
			exitf("watch config: %v", err)
		}
		return
	}

	config, err := core.LoadConfig(*configPath)
	if err != nil {
		exitf("load config: %v", err)
	}

	runtime, err := core.NewRuntime(manager, config)
	if err != nil {
		exitf("load plugins: %v", err)
	}

	if err := printResult(runtime.Run(map[string]interface{}{"input": *input})); err != nil {
		exitf("encode result: %v", err)
	}
}

func printResult(result core.ExecutionResult) error {
	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(output))
	return nil
}

func exitf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
