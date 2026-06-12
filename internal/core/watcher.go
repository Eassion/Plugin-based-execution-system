package core

import (
	"context"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

const configReloadDebounce = 100 * time.Millisecond

func WatchConfig(ctx context.Context, path string, onReload func(Config) error) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	cleanPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	dir := filepath.Dir(cleanPath)
	if err := watcher.Add(dir); err != nil {
		return err
	}

	if err := reloadConfig(cleanPath, onReload); err != nil {
		return err
	}

	var timer *time.Timer
	var timerC <-chan time.Time
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()

	scheduleReload := func() {
		if timer == nil {
			timer = time.NewTimer(configReloadDebounce)
			timerC = timer.C
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(configReloadDebounce)
		timerC = timer.C
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			eventPath, err := filepath.Abs(event.Name)
			if err != nil || eventPath != cleanPath {
				continue
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Rename) {
				scheduleReload()
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			return err
		case <-timerC:
			timerC = nil
			if err := reloadConfig(cleanPath, onReload); err != nil {
				return err
			}
		}
	}
}

func reloadConfig(path string, onReload func(Config) error) error {
	config, err := LoadConfig(path)
	if err != nil {
		return err
	}
	return onReload(config)
}
