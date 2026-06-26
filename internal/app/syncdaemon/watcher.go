package syncdaemon

import (
	"context"
	"io/fs"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

type WatchEvent struct{ Path string }

type Watcher interface {
	Events() <-chan WatchEvent
	Errors() <-chan error
	Close() error
}

type fsnotifyWatcher struct {
	w      *fsnotify.Watcher
	events chan WatchEvent
	errors chan error
}

func NewFSNotifyWatcher(root string) (Watcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	out := &fsnotifyWatcher{w: w, events: make(chan WatchEvent, 32), errors: make(chan error, 1)}
	go out.forward()
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr == nil && IgnoreRuntimePath(rel) {
			return filepath.SkipDir
		}
		return w.Add(path)
	})
	if err != nil {
		_ = w.Close()
		return nil, err
	}
	return out, nil
}

func (w *fsnotifyWatcher) Events() <-chan WatchEvent { return w.events }
func (w *fsnotifyWatcher) Errors() <-chan error      { return w.errors }
func (w *fsnotifyWatcher) Close() error              { return w.w.Close() }

func (w *fsnotifyWatcher) forward() {
	defer close(w.events)
	defer close(w.errors)
	for {
		select {
		case event, ok := <-w.w.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename) {
				w.events <- WatchEvent{Path: event.Name}
			}
		case err, ok := <-w.w.Errors:
			if !ok {
				return
			}
			w.errors <- err
		}
	}
}

func Debounce(ctx context.Context, in <-chan WatchEvent, delay time.Duration) <-chan []WatchEvent {
	if delay <= 0 {
		delay = 250 * time.Millisecond
	}
	out := make(chan []WatchEvent, 1)
	go func() {
		defer close(out)
		var batch []WatchEvent
		var timer *time.Timer
		var timerC <-chan time.Time
		flush := func() {
			if len(batch) == 0 {
				return
			}
			out <- coalesce(batch)
			batch = nil
		}
		for {
			select {
			case <-ctx.Done():
				flush()
				return
			case event, ok := <-in:
				if !ok {
					flush()
					return
				}
				if IgnoreRuntimePath(event.Path) {
					continue
				}
				batch = append(batch, event)
				if timer != nil {
					timer.Stop()
				}
				timer = time.NewTimer(delay)
				timerC = timer.C
			case <-timerC:
				flush()
				timerC = nil
			}
		}
	}()
	return out
}

func coalesce(events []WatchEvent) []WatchEvent {
	seen := map[string]bool{}
	out := make([]WatchEvent, 0, len(events))
	for _, event := range events {
		path := SafeEventPath(event.Path)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, WatchEvent{Path: path})
	}
	return out
}

type Backoff struct {
	Base    time.Duration
	Max     time.Duration
	Attempt int
}

func (b *Backoff) Next(now time.Time) time.Time {
	base := b.Base
	if base <= 0 {
		base = time.Second
	}
	maxDelay := b.Max
	if maxDelay <= 0 {
		maxDelay = 30 * time.Second
	}
	delay := base << b.Attempt
	if delay > maxDelay {
		delay = maxDelay
	}
	b.Attempt++
	return now.Add(delay)
}

func (b *Backoff) Reset() { b.Attempt = 0 }
