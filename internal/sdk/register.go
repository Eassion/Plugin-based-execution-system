package sdk

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"plugin-execution-system/internal/core"
)

func Register(ctx context.Context, brokerRegisterPath string, meta core.PluginMeta) error {
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "unix", brokerRegisterPath)
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(core.RegisterRequest{Meta: meta}); err != nil {
		return err
	}

	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return err
	}
	var response core.RegisterResponse
	if err := json.Unmarshal(line, &response); err != nil {
		return err
	}
	if !response.Success {
		return fmt.Errorf(response.Error)
	}
	return nil
}

func timeUnixMilli(value int64) time.Time {
	return time.Unix(0, value*int64(time.Millisecond))
}

type coreError string

func (e coreError) Error() string {
	return string(e)
}
