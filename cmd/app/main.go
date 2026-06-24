package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"plugin-execution-system/internal/core"
)

func main() {
	configPath := flag.String("config", "plugins.json", "plugin config file")
	input := flag.String("input", "hello plugin system", "input text")
	watch := flag.Bool("watch", false, "watch plugin config and reload execution plan")
	flag.Parse()

	config, err := core.LoadConfig(*configPath)
	if err != nil {
		exitf("load config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	broker, cleanup, err := startBrokerWithWorkers(ctx, config)
	if err != nil {
		exitf("start broker: %v", err)
	}
	defer cleanup()

	manager := brokerManager(broker)

	if *watch {
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

func startBrokerWithWorkers(ctx context.Context, config core.Config) (*core.Broker, func(), error) {
	broker := core.NewBroker()
	registerPath := shortSocketPath("broker-register")
	_ = os.Remove(registerPath)

	listener, err := net.Listen("unix", registerPath)
	if err != nil {
		return nil, nil, err
	}

	serverCtx, serverStop := context.WithCancel(ctx)
	errs := make(chan error, 1)
	go func() {
		errs <- broker.ServeRegistration(serverCtx, listener)
	}()

	processes := make([]*exec.Cmd, 0)
	started := make(map[string]bool)
	for _, pluginConfig := range config.Plugins {
		commandPath, ok := defaultPluginCommands()[pluginConfig.Name]
		if !ok || started[pluginConfig.Name] {
			continue
		}
		started[pluginConfig.Name] = true
		cmd := exec.CommandContext(ctx, "go", "run", commandPath)
		cmd.Env = append(os.Environ(),
			"BROKER_REGISTER_ADDR="+registerPath,
			"PLUGIN_UDS_PATH="+shortSocketPath(pluginConfig.Name),
			"PLUGIN_DEPENDS_ON="+strings.Join(pluginConfig.DependsOn, ","),
		)
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			serverStop()
			return nil, nil, err
		}
		processes = append(processes, cmd)
	}

	if err := waitForRegisteredPlugins(ctx, broker, config); err != nil {
		serverStop()
		return nil, nil, err
	}

	cleanup := func() {
		serverStop()
		_ = os.Remove(registerPath)
		for _, cmd := range processes {
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			_ = cmd.Wait()
		}
		select {
		case <-errs:
		default:
		}
	}
	return broker, cleanup, nil
}

func brokerManager(broker *core.Broker) *core.Manager {
	manager := core.NewManager()
	for _, info := range broker.ListPlugins() {
		_ = manager.Register(core.NewBrokerPlugin(info.PluginID, info.Version, broker))
	}
	return manager
}

func waitForRegisteredPlugins(ctx context.Context, broker *core.Broker, config core.Config) error {
	required := make(map[string]bool)
	for _, pluginConfig := range config.Plugins {
		if pluginConfig.Enabled {
			required[pluginConfig.Name] = true
		}
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		allRegistered := true
		for name := range required {
			if _, ok := broker.GetPlugin(name); !ok {
				allRegistered = false
				break
			}
		}
		if allRegistered {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	return fmt.Errorf("timed out waiting for plugin registration")
}

func defaultPluginCommands() map[string]string {
	return map[string]string{
		"uppercase": "./cmd/plugin-uppercase",
		"wordcount": "./cmd/plugin-wordcount",
		"timestamp": "./cmd/plugin-timestamp",
	}
}

func shortSocketPath(name string) string {
	cleanName := strings.ReplaceAll(name, string(os.PathSeparator), "-")
	return filepath.Join(os.TempDir(), fmt.Sprintf("pes-%s-%d.sock", cleanName, time.Now().UnixNano()))
}
