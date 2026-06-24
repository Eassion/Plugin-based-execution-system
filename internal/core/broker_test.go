package core_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"plugin-execution-system/internal/core"
)

func TestBrokerRejectsUnreachablePluginWithoutRegistering(t *testing.T) {
	broker := core.NewBroker()

	err := broker.Register(context.Background(), core.PluginMeta{
		PluginID: "missing",
		Version:  "1.0.0",
		UDSPath:  filepath.Join(t.TempDir(), "missing.sock"),
	})

	if err == nil {
		t.Fatal("expected unreachable plugin error")
	}
	if _, ok := broker.GetPlugin("missing"); ok {
		t.Fatal("unreachable plugin should not be registered")
	}
}

func TestBrokerRegistersReachablePluginAndListsMetadata(t *testing.T) {
	socketPath, stop := startTestPluginServer(t, func(data map[string]interface{}) (map[string]interface{}, error) {
		data["registered"] = true
		return data, nil
	})
	defer stop()

	broker := core.NewBroker()
	err := broker.Register(context.Background(), core.PluginMeta{
		PluginID:    "uppercase",
		Version:     "1.0.0",
		UDSPath:     socketPath,
		Description: "test plugin",
	})
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	info, ok := broker.GetPlugin("uppercase")
	if !ok {
		t.Fatal("expected plugin metadata")
	}
	if info.PluginID != "uppercase" || info.Version != "1.0.0" || !info.Enabled || !info.Healthy {
		t.Fatalf("unexpected plugin info: %#v", info)
	}
}

func TestBrokerRollsBackDependencyGraphOnCircularDependency(t *testing.T) {
	aPath, stopA := startTestPluginServer(t, passJSON)
	defer stopA()
	bPath, stopB := startTestPluginServer(t, passJSON)
	defer stopB()

	broker := core.NewBroker()
	if err := broker.Register(context.Background(), core.PluginMeta{
		PluginID: "a",
		Version:  "1.0.0",
		UDSPath:  aPath,
	}); err != nil {
		t.Fatalf("register a: %v", err)
	}

	err := broker.Register(context.Background(), core.PluginMeta{
		PluginID:  "b",
		Version:   "1.0.0",
		DependsOn: []string{"a"},
		UDSPath:   bPath,
	})
	if err != nil {
		t.Fatalf("register b: %v", err)
	}

	a2Path, stopA2 := startTestPluginServer(t, passJSON)
	defer stopA2()
	err = broker.Register(context.Background(), core.PluginMeta{
		PluginID:  "a",
		Version:   "2.0.0",
		DependsOn: []string{"b"},
		UDSPath:   a2Path,
	})
	if err == nil {
		t.Fatal("expected circular dependency error")
	}

	info, ok := broker.GetPlugin("a")
	if !ok || info.Version != "1.0.0" {
		t.Fatalf("expected old plugin metadata after rollback, got %#v", info)
	}
}

func TestBrokerInvokeHonorsEnabledFlagWithoutDisconnecting(t *testing.T) {
	socketPath, stop := startTestPluginServer(t, passJSON)
	defer stop()

	broker := core.NewBroker()
	if err := broker.Register(context.Background(), core.PluginMeta{PluginID: "p", Version: "1.0.0", UDSPath: socketPath}); err != nil {
		t.Fatalf("register: %v", err)
	}

	broker.SetEnabled("p", false)
	_, err := broker.Invoke(context.Background(), "p", map[string]interface{}{"input": "hello"})
	if err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("expected disabled error, got %v", err)
	}

	broker.SetEnabled("p", true)
	result, err := broker.Invoke(context.Background(), "p", map[string]interface{}{"input": "hello"})
	if err != nil {
		t.Fatalf("Invoke returned error after re-enable: %v", err)
	}
	if result["input"] != "hello" {
		t.Fatalf("unexpected response: %#v", result)
	}
}

func TestBrokerCircuitBreakerRejectsAfterFiveFailures(t *testing.T) {
	socketPath, stop := startTestPluginServer(t, func(data map[string]interface{}) (map[string]interface{}, error) {
		return nil, errTestPlugin
	})
	defer stop()

	broker := core.NewBroker()
	if err := broker.Register(context.Background(), core.PluginMeta{PluginID: "p", Version: "1.0.0", UDSPath: socketPath}); err != nil {
		t.Fatalf("register: %v", err)
	}

	for i := 0; i < 5; i++ {
		if _, err := broker.Invoke(context.Background(), "p", map[string]interface{}{}); err == nil {
			t.Fatalf("expected failure %d", i+1)
		}
	}

	_, err := broker.Invoke(context.Background(), "p", map[string]interface{}{})
	if err == nil || !strings.Contains(err.Error(), "circuit breaker") {
		t.Fatalf("expected circuit breaker error, got %v", err)
	}
}

func TestBrokerInvokeRejectsExpiredContext(t *testing.T) {
	socketPath, stop := startTestPluginServer(t, passJSON)
	defer stop()

	broker := core.NewBroker()
	if err := broker.Register(context.Background(), core.PluginMeta{PluginID: "p", Version: "1.0.0", UDSPath: socketPath}); err != nil {
		t.Fatalf("register: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := broker.Invoke(ctx, "p", map[string]interface{}{})
	if err == nil {
		t.Fatal("expected context error")
	}
}

func TestPipelineRunsBrokerBackedPlugin(t *testing.T) {
	socketPath, stop := startTestPluginServer(t, func(data map[string]interface{}) (map[string]interface{}, error) {
		data["input"] = strings.ToUpper(data["input"].(string))
		return data, nil
	})
	defer stop()

	broker := core.NewBroker()
	if err := broker.Register(context.Background(), core.PluginMeta{PluginID: "uppercase", Version: "1.0.0", UDSPath: socketPath}); err != nil {
		t.Fatalf("register: %v", err)
	}

	pipeline := core.NewPipeline([]core.ConfiguredPlugin{
		{
			Plugin: core.NewBrokerPlugin("uppercase", "1.0.0", broker),
			Config: core.PluginConfig{Name: "uppercase", Enabled: true, TimeoutMS: 1000},
		},
	}, nil)

	result := pipeline.Run(map[string]interface{}{"input": "hello"})

	if result.Data["input"] != "HELLO" {
		t.Fatalf("expected broker-backed plugin to update data, got %#v", result.Data)
	}
	if len(result.PluginResults) != 1 || result.PluginResults[0].Status != core.StatusSuccess {
		t.Fatalf("unexpected plugin results: %#v", result.PluginResults)
	}
}

var errTestPlugin = &testPluginError{"boom"}

type testPluginError struct {
	msg string
}

func (e *testPluginError) Error() string {
	return e.msg
}

func passJSON(data map[string]interface{}) (map[string]interface{}, error) {
	return data, nil
}

func startTestPluginServer(t *testing.T, handler func(map[string]interface{}) (map[string]interface{}, error)) (string, func()) {
	t.Helper()

	socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("pes-%d.sock", time.Now().UnixNano()))
	_ = os.Remove(socketPath)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen unix socket: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go serveTestPluginConn(conn, handler)
		}
	}()

	return socketPath, func() {
		_ = listener.Close()
		_ = os.Remove(socketPath)
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("test plugin server did not stop")
		}
	}
}

func serveTestPluginConn(conn net.Conn, handler func(map[string]interface{}) (map[string]interface{}, error)) {
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	encoder := json.NewEncoder(conn)
	for scanner.Scan() {
		var request core.IPCRequest
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			_ = encoder.Encode(core.IPCResponse{RequestID: request.RequestID, Success: false, Error: err.Error()})
			continue
		}
		data, err := handler(request.Data)
		response := core.IPCResponse{RequestID: request.RequestID, Success: true, Data: data}
		if err != nil {
			response.Success = false
			response.Error = err.Error()
		}
		_ = encoder.Encode(response)
	}
}
