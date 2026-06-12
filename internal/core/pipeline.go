package core

import (
	"context"
	"fmt"
	"reflect"
	"time"
)

const (
	DefaultPluginTimeoutMS = 30000
	MaxPluginTimeoutMS     = 60000
)

type Status string

const (
	StatusSuccess Status = "success"
	StatusFailed  Status = "failed"
	StatusSkipped Status = "skipped"
)

type ExecutionResult struct {
	Data          map[string]interface{} `json:"data"`
	PluginResults []PluginResult         `json:"plugin_results"`
}

type PluginResult struct {
	Name       string `json:"name"`
	Version    string `json:"version,omitempty"`
	Enabled    bool   `json:"enabled"`
	Status     Status `json:"status"`
	DurationMS int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
}

type Pipeline struct {
	plugins []ConfiguredPlugin
	configs []PluginConfig
}

func NewPipeline(plugins []ConfiguredPlugin, configs []PluginConfig) Pipeline {
	return Pipeline{plugins: plugins, configs: configs}
}

func (p Pipeline) Run(data map[string]interface{}) ExecutionResult {
	current := cloneMap(data)
	results := make([]PluginResult, 0, len(p.plugins)+len(p.configs))

	for _, config := range p.configs {
		if config.Enabled {
			continue
		}
		results = append(results, PluginResult{
			Name:    config.Name,
			Enabled: false,
			Status:  StatusSkipped,
		})
	}

	for _, item := range p.plugins {
		plugin := item.Plugin
		start := time.Now()
		next, err := runPlugin(item, current)
		result := PluginResult{
			Name:       plugin.Name(),
			Version:    plugin.Version(),
			Enabled:    true,
			Status:     StatusSuccess,
			DurationMS: time.Since(start).Milliseconds(),
		}
		if err != nil {
			result.Status = StatusFailed
			result.Error = err.Error()
			results = append(results, result)
			continue
		}

		current = next
		results = append(results, result)
	}

	return ExecutionResult{
		Data:          current,
		PluginResults: results,
	}
}

func runPlugin(item ConfiguredPlugin, current map[string]interface{}) (map[string]interface{}, error) {
	type pluginOutput struct {
		data map[string]interface{}
		err  error
	}

	timeoutMS := item.Config.TimeoutMS
	if timeoutMS <= 0 {
		timeoutMS = DefaultPluginTimeoutMS
	}
	if timeoutMS > MaxPluginTimeoutMS {
		return current, fmt.Errorf("plugin timeout %dms exceeds maximum %dms", timeoutMS, MaxPluginTimeoutMS)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMS)*time.Millisecond)
	defer cancel()

	output := make(chan pluginOutput, 1)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				output <- pluginOutput{data: current, err: fmt.Errorf("plugin panic: %v", recovered)}
			}
		}()
		next, err := item.Plugin.Run(ctx, cloneMap(current))
		output <- pluginOutput{data: next, err: err}
	}()

	select {
	case result := <-output:
		return result.data, result.err
	case <-ctx.Done():
		return current, fmt.Errorf("plugin timed out after %dms", timeoutMS)
	}
}

func cloneMap(data map[string]interface{}) map[string]interface{} {
	cloned := make(map[string]interface{}, len(data))
	for key, value := range data {
		cloned[key] = deepClone(value)
	}
	return cloned
}

func deepClone(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		return cloneMap(typed)
	case []interface{}:
		cloned := make([]interface{}, len(typed))
		for i, item := range typed {
			cloned[i] = deepClone(item)
		}
		return cloned
	}

	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() {
		return value
	}
	switch reflected.Kind() {
	case reflect.Slice:
		cloned := reflect.MakeSlice(reflected.Type(), reflected.Len(), reflected.Len())
		reflect.Copy(cloned, reflected)
		return cloned.Interface()
	case reflect.Map:
		cloned := reflect.MakeMapWithSize(reflected.Type(), reflected.Len())
		for _, key := range reflected.MapKeys() {
			cloned.SetMapIndex(key, reflected.MapIndex(key))
		}
		return cloned.Interface()
	default:
		return value
	}
}
