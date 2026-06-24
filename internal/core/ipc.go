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

type IPCRequest struct {
	RequestID      string                 `json:"request_id"`
	PluginID       string                 `json:"plugin_id"`
	DeadlineUnixMS int64                  `json:"deadline_unix_ms,omitempty"`
	Data           map[string]interface{} `json:"data"`
}

type IPCResponse struct {
	RequestID string                 `json:"request_id"`
	Success   bool                   `json:"success"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Error     string                 `json:"error,omitempty"`
}

type PluginClient interface {
	Invoke(ctx context.Context, pluginID string, data map[string]interface{}) (map[string]interface{}, error)
}

type JSONPluginClient struct {
	mu      sync.Mutex
	conn    net.Conn
	reader  *bufio.Reader
	encoder *json.Encoder
	nextID  uint64
}

func NewJSONPluginClient(conn net.Conn) *JSONPluginClient {
	return &JSONPluginClient{
		conn:    conn,
		reader:  bufio.NewReader(conn),
		encoder: json.NewEncoder(conn),
	}
}

func (c *JSONPluginClient) Invoke(ctx context.Context, pluginID string, data map[string]interface{}) (map[string]interface{}, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if deadline, ok := ctx.Deadline(); ok {
		if err := c.conn.SetDeadline(deadline); err != nil {
			return nil, err
		}
		defer c.conn.SetDeadline(time.Time{})
	}

	requestID := fmt.Sprintf("%s-%d", pluginID, atomic.AddUint64(&c.nextID, 1))
	request := IPCRequest{
		RequestID: requestID,
		PluginID:  pluginID,
		Data:      cloneMap(data),
	}
	if deadline, ok := ctx.Deadline(); ok {
		request.DeadlineUnixMS = deadline.UnixMilli()
	}

	if err := c.encoder.Encode(request); err != nil {
		return nil, err
	}

	line, err := c.reader.ReadBytes('\n')
	if err != nil {
		return nil, err
	}

	var response IPCResponse
	if err := json.Unmarshal(line, &response); err != nil {
		return nil, err
	}
	if response.RequestID != requestID {
		return nil, fmt.Errorf("unexpected response id %q for request %q", response.RequestID, requestID)
	}
	if !response.Success {
		if response.Error == "" {
			response.Error = "plugin returned failure"
		}
		return nil, fmt.Errorf(response.Error)
	}
	return cloneMap(response.Data), nil
}
