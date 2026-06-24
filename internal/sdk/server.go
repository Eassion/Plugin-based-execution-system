package sdk

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os"

	"plugin-execution-system/internal/core"
)

type Handler func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error)

type Server struct {
	PluginID string
	Handler  Handler
}

func (s Server) Serve(ctx context.Context, udsPath string) error {
	if s.PluginID == "" {
		return coreError("plugin id is required")
	}
	if s.Handler == nil {
		return coreError("handler is required")
	}

	_ = os.Remove(udsPath)
	listener, err := net.Listen("unix", udsPath)
	if err != nil {
		return err
	}
	defer listener.Close()
	defer os.Remove(udsPath)

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return err
			}
		}
		go s.serveConn(ctx, conn)
	}
}

func (s Server) serveConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	encoder := json.NewEncoder(conn)
	for scanner.Scan() {
		var request core.IPCRequest
		response := core.IPCResponse{Success: true}
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			response.Success = false
			response.Error = err.Error()
			_ = encoder.Encode(response)
			continue
		}
		response.RequestID = request.RequestID
		callCtx := ctx
		if request.DeadlineUnixMS > 0 {
			var cancel context.CancelFunc
			callCtx, cancel = context.WithDeadline(ctx, timeUnixMilli(request.DeadlineUnixMS))
			defer cancel()
		}
		data, err := s.Handler(callCtx, request.Data)
		response.Data = data
		if err != nil {
			response.Success = false
			response.Error = err.Error()
		}
		_ = encoder.Encode(response)
	}
}
