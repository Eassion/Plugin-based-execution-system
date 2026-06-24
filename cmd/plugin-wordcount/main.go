package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"plugin-execution-system/internal/core"
	"plugin-execution-system/internal/sdk"
)

func main() {
	runPlugin("wordcount", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
		input, err := stringInput(data)
		if err != nil {
			return data, err
		}
		data["word_count"] = len(strings.Fields(input))
		return data, nil
	})
}

func runPlugin(pluginID string, handler sdk.Handler) {
	brokerPath := os.Getenv("BROKER_REGISTER_ADDR")
	udsPath := os.Getenv("PLUGIN_UDS_PATH")
	if brokerPath == "" || udsPath == "" {
		fmt.Fprintln(os.Stderr, "BROKER_REGISTER_ADDR and PLUGIN_UDS_PATH are required")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	server := sdk.Server{PluginID: pluginID, Handler: handler}
	errs := make(chan error, 1)
	go func() {
		errs <- server.Serve(ctx, udsPath)
	}()

	if err := waitForSocket(ctx, udsPath); err != nil {
		fmt.Fprintf(os.Stderr, "wait for plugin socket: %v\n", err)
		os.Exit(1)
	}

	meta := core.PluginMeta{
		PluginID:    pluginID,
		Version:     "1.0.0",
		DependsOn:   splitList(os.Getenv("PLUGIN_DEPENDS_ON")),
		UDSPath:     udsPath,
		Description: pluginID + " plugin",
	}
	if err := sdk.Register(ctx, brokerPath, meta); err != nil {
		fmt.Fprintf(os.Stderr, "register plugin: %v\n", err)
		os.Exit(1)
	}

	if err := <-errs; err != nil {
		fmt.Fprintf(os.Stderr, "serve plugin: %v\n", err)
		os.Exit(1)
	}
}

func stringInput(data map[string]interface{}) (string, error) {
	value, ok := data["input"]
	if !ok {
		return "", fmt.Errorf("input field is required")
	}
	input, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("input field must be a string")
	}
	return input, nil
}

func splitList(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			items = append(items, part)
		}
	}
	return items
}

func waitForSocket(ctx context.Context, path string) error {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := netDial("unix", path)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
	return fmt.Errorf("timed out waiting for %s", path)
}
