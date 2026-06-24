package sdk_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"plugin-execution-system/internal/core"
	"plugin-execution-system/internal/sdk"
)

func TestServerHandlesJSONRequests(t *testing.T) {
	socketPath := shortSocketPath(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server := sdk.Server{
		PluginID: "uppercase",
		Handler: func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
			data["handled"] = true
			return data, nil
		},
	}
	go func() {
		_ = server.Serve(ctx, socketPath)
	}()
	waitForSocket(t, socketPath)

	broker := core.NewBroker()
	if err := broker.Register(context.Background(), core.PluginMeta{PluginID: "uppercase", Version: "1.0.0", UDSPath: socketPath}); err != nil {
		t.Fatalf("register: %v", err)
	}

	result, err := broker.Invoke(context.Background(), "uppercase", map[string]interface{}{"input": "hello"})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if result["handled"] != true {
		t.Fatalf("expected handler to update response, got %#v", result)
	}
}

func TestRegisterSendsMetadataToBrokerRegistrationSocket(t *testing.T) {
	brokerSocket := shortSocketPath(t)
	pluginSocket := shortSocketPath(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	broker := core.NewBroker()
	listener, err := net.Listen("unix", brokerSocket)
	if err != nil {
		t.Fatalf("listen broker: %v", err)
	}
	go func() {
		_ = broker.ServeRegistration(ctx, listener)
	}()
	waitForSocket(t, brokerSocket)

	server := sdk.Server{PluginID: "p", Handler: func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
		return data, nil
	}}
	go func() {
		_ = server.Serve(ctx, pluginSocket)
	}()
	waitForSocket(t, pluginSocket)

	err = sdk.Register(context.Background(), brokerSocket, core.PluginMeta{
		PluginID: "p",
		Version:  "1.0.0",
		UDSPath:  pluginSocket,
	})
	if err != nil {
		t.Fatalf("sdk register: %v", err)
	}

	if _, ok := broker.GetPlugin("p"); !ok {
		t.Fatal("expected broker to register plugin metadata")
	}
}

func waitForSocket(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.Dial("unix", path)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for socket %s", path)
}

func shortSocketPath(t *testing.T) string {
	t.Helper()
	path := filepath.Join(os.TempDir(), fmt.Sprintf("pes-sdk-%d.sock", time.Now().UnixNano()))
	_ = os.Remove(path)
	t.Cleanup(func() {
		_ = os.Remove(path)
	})
	return path
}
