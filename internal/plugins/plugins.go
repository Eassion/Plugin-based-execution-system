package plugins

import (
	"context"
	"fmt"
	"strings"
	"time"

	"plugin-execution-system/internal/core"
)

func Uppercase() core.Plugin {
	return core.NewPlugin("uppercase", "1.0.0", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
		if err := ctx.Err(); err != nil {
			return data, err
		}
		input, err := stringInput(data)
		if err != nil {
			return data, err
		}
		data["input"] = strings.ToUpper(input)
		return data, nil
	})
}

func WordCount() core.Plugin {
	return core.NewPlugin("wordcount", "1.0.0", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
		if err := ctx.Err(); err != nil {
			return data, err
		}
		input, err := stringInput(data)
		if err != nil {
			return data, err
		}
		data["word_count"] = len(strings.Fields(input))
		return data, nil
	})
}

func Timestamp() core.Plugin {
	return core.NewPlugin("timestamp", "1.0.0", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
		if err := ctx.Err(); err != nil {
			return data, err
		}
		data["processed_at"] = time.Now().UTC().Format(time.RFC3339)
		return data, nil
	})
}

func RegisterDefaults(manager *core.Manager) {
	_ = manager.Register(Uppercase())
	_ = manager.Register(WordCount())
	_ = manager.Register(Timestamp())
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
