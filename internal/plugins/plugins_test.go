package plugins_test

import (
	"context"
	"testing"

	"plugin-execution-system/internal/plugins"
)

func TestUppercasePluginTransformsInput(t *testing.T) {
	plugin := plugins.Uppercase()

	data, err := plugin.Run(context.Background(), map[string]interface{}{"input": "hello world"})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if data["input"] != "HELLO WORLD" {
		t.Fatalf("expected uppercase input, got %#v", data["input"])
	}
}

func TestWordCountPluginAddsWordCount(t *testing.T) {
	plugin := plugins.WordCount()

	data, err := plugin.Run(context.Background(), map[string]interface{}{"input": "hello plugin system"})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if data["word_count"] != 3 {
		t.Fatalf("expected word_count 3, got %#v", data["word_count"])
	}
}

func TestTimestampPluginAddsProcessedAt(t *testing.T) {
	plugin := plugins.Timestamp()

	data, err := plugin.Run(context.Background(), map[string]interface{}{"input": "hello"})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if data["processed_at"] == "" || data["processed_at"] == nil {
		t.Fatalf("expected processed_at to be populated, got %#v", data["processed_at"])
	}
}
