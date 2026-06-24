package core

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultRegisterDialTimeout = time.Second
	defaultDrainTimeout        = 10 * time.Second
)

type PluginMeta struct {
	PluginID    string   `json:"plugin_id"`
	Version     string   `json:"version"`
	DependsOn   []string `json:"depends_on,omitempty"`
	UDSPath     string   `json:"uds_path"`
	Description string   `json:"description,omitempty"`
}

type RegisterRequest struct {
	Meta PluginMeta `json:"meta"`
}

type RegisterResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

type PluginRuntime struct {
	Meta     PluginMeta
	Conn     net.Conn
	Client   PluginClient
	Enabled  bool
	Healthy  bool
	Draining bool
	Breaker  *CircuitBreaker

	activeCount int64
}

type Broker struct {
	mu       sync.RWMutex
	registry map[string]*PluginRuntime
	deps     *DependencyGraph
}

func NewBroker() *Broker {
	return &Broker{
		registry: make(map[string]*PluginRuntime),
		deps:     NewDependencyGraph(),
	}
}

func (b *Broker) ServeRegistration(ctx context.Context, listener net.Listener) error {
	errs := make(chan error, 1)
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				select {
				case <-ctx.Done():
					errs <- nil
				default:
					errs <- err
				}
				return
			}
			go b.serveRegisterConn(ctx, conn)
		}
	}()

	return <-errs
}

func (b *Broker) serveRegisterConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	encoder := json.NewEncoder(conn)
	for scanner.Scan() {
		var request RegisterRequest
		response := RegisterResponse{Success: true}
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			response.Success = false
			response.Error = err.Error()
			_ = encoder.Encode(response)
			continue
		}
		if err := b.Register(ctx, request.Meta); err != nil {
			response.Success = false
			response.Error = err.Error()
		}
		_ = encoder.Encode(response)
	}
}

func (b *Broker) Register(ctx context.Context, meta PluginMeta) error {
	if meta.PluginID == "" {
		return fmt.Errorf("plugin_id is required")
	}
	if meta.UDSPath == "" {
		return fmt.Errorf("uds_path is required")
	}

	dialCtx, cancel := context.WithTimeout(ctx, defaultRegisterDialTimeout)
	defer cancel()

	dialer := net.Dialer{}
	conn, err := dialer.DialContext(dialCtx, "unix", meta.UDSPath)
	if err != nil {
		return fmt.Errorf("dial plugin %q at %q: %w", meta.PluginID, meta.UDSPath, err)
	}

	runtime := &PluginRuntime{
		Meta:    meta,
		Conn:    conn,
		Client:  NewJSONPluginClient(conn),
		Enabled: true,
		Healthy: true,
		Breaker: NewCircuitBreaker(),
	}

	var oldRuntime *PluginRuntime
	b.mu.Lock()
	snapshot := b.deps.Snapshot()
	oldRuntime = b.registry[meta.PluginID]
	b.deps.Put(meta.PluginID, meta.DependsOn)
	if err := b.deps.ValidateAcyclic(); err != nil {
		b.deps.Restore(snapshot)
		b.mu.Unlock()
		_ = conn.Close()
		return err
	}
	b.registry[meta.PluginID] = runtime
	if oldRuntime != nil {
		oldRuntime.Draining = true
	}
	b.mu.Unlock()

	if oldRuntime != nil {
		go drainRuntime(oldRuntime)
	}
	return nil
}

func (b *Broker) Invoke(ctx context.Context, pluginID string, data map[string]interface{}) (map[string]interface{}, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	b.mu.RLock()
	runtime, ok := b.registry[pluginID]
	if !ok {
		b.mu.RUnlock()
		return nil, fmt.Errorf("plugin %q is not registered", pluginID)
	}
	if !runtime.Enabled {
		b.mu.RUnlock()
		return nil, fmt.Errorf("plugin %q is disabled", pluginID)
	}
	if !runtime.Healthy || runtime.Draining {
		b.mu.RUnlock()
		return nil, fmt.Errorf("plugin %q is not healthy", pluginID)
	}
	if err := runtime.Breaker.Allow(); err != nil {
		b.mu.RUnlock()
		return nil, fmt.Errorf("plugin %q %w", pluginID, err)
	}
	atomic.AddInt64(&runtime.activeCount, 1)
	b.mu.RUnlock()

	defer atomic.AddInt64(&runtime.activeCount, -1)

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result, err := runtime.Client.Invoke(ctx, pluginID, data)
	if err != nil {
		runtime.Breaker.RecordFailure()
		return nil, err
	}
	runtime.Breaker.RecordSuccess()
	return result, nil
}

func (b *Broker) SetEnabled(pluginID string, enabled bool) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	runtime, ok := b.registry[pluginID]
	if !ok {
		return fmt.Errorf("plugin %q is not registered", pluginID)
	}
	runtime.Enabled = enabled
	return nil
}

func (b *Broker) GetPlugin(pluginID string) (PluginInfo, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	runtime, ok := b.registry[pluginID]
	if !ok {
		return PluginInfo{}, false
	}
	return runtime.info(), true
}

func (b *Broker) ListPlugins() []PluginInfo {
	b.mu.RLock()
	defer b.mu.RUnlock()

	infos := make([]PluginInfo, 0, len(b.registry))
	for _, runtime := range b.registry {
		infos = append(infos, runtime.info())
	}
	return infos
}

func (r *PluginRuntime) info() PluginInfo {
	return PluginInfo{
		Name:        r.Meta.PluginID,
		PluginID:    r.Meta.PluginID,
		Version:     r.Meta.Version,
		DependsOn:   append([]string(nil), r.Meta.DependsOn...),
		UDSPath:     r.Meta.UDSPath,
		Description: r.Meta.Description,
		Enabled:     r.Enabled,
		Healthy:     r.Healthy,
		Draining:    r.Draining,
	}
}

func drainRuntime(runtime *PluginRuntime) {
	deadline := time.Now().Add(defaultDrainTimeout)
	for {
		if atomic.LoadInt64(&runtime.activeCount) == 0 || time.Now().After(deadline) {
			_ = runtime.Conn.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}
