package config

import (
	"context"
	"time"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
)

// Watcher watches a config file for changes and calls onChange when the file is modified.
// It uses fsnotify and debounces rapid successive writes with a 200ms delay.
type Watcher struct {
	path     string
	onChange func(*Config)
	log      *zap.Logger
	fsw      *fsnotify.Watcher
}

// NewWatcher creates a new Watcher for the given config file path.
func NewWatcher(path string, onChange func(*Config), log *zap.Logger) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := fsw.Add(path); err != nil {
		fsw.Close()
		return nil, err
	}
	return &Watcher{
		path:     path,
		onChange: onChange,
		log:      log,
		fsw:      fsw,
	}, nil
}

// Start runs the file watch loop in a background goroutine.
// It returns immediately. The loop exits when ctx is cancelled or Stop is called.
func (w *Watcher) Start(ctx context.Context) {
	go w.loop(ctx)
}

// Stop shuts down the underlying fsnotify watcher.
func (w *Watcher) Stop() {
	w.fsw.Close()
}

func (w *Watcher) loop(ctx context.Context) {
	var debounce *time.Timer

	for {
		select {
		case <-ctx.Done():
			if debounce != nil {
				debounce.Stop()
			}
			return

		case event, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			// We care about write and create events (editors often use rename-then-move)
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Rename) {
				// Re-add the path in case it was renamed (some editors replace the file)
				_ = w.fsw.Add(w.path)

				// Debounce: reset timer to 200ms
				if debounce != nil {
					debounce.Stop()
				}
				debounce = time.AfterFunc(200*time.Millisecond, func() {
					cfg, err := Load(w.path)
					if err != nil {
						w.log.Warn("config reload failed", zap.String("path", w.path), zap.Error(err))
						return
					}
					w.log.Info("config reloaded", zap.String("path", w.path))
					w.onChange(cfg)
				})
			}

		case err, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			w.log.Warn("fsnotify error", zap.Error(err))
		}
	}
}
