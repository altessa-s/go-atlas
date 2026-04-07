// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugins

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	coretime "github.com/altessa-s/go-atlas/core/time"
)

// IsWatching reports whether the plugin directory watcher is currently
// running. It is safe to call concurrently with [Manager.StartWatching] and
// [Manager.StopWatching].
func (m *Manager) IsWatching() bool {
	m.watchMu.Lock()
	defer m.watchMu.Unlock()
	return m.watching
}

// StartWatching begins monitoring the configured plugin directory for newly
// added .so files and loads them automatically. The watcher coalesces rapid
// filesystem events using the configured debounce window and delegates the
// actual load work to [Manager.Reload], which is idempotent with respect to
// already-loaded plugins.
//
// Only CREATE and WRITE events on .so files are acted upon. Modifications to
// already-loaded plugins are ignored because Go's plugin package cannot
// unload or replace code that has been mapped into the process; to upgrade
// a plugin the application must be restarted. Removals are also ignored for
// the same reason.
//
// StartWatching is a no-op if the watcher is already running. It returns
// [ErrManagerClosed] if the manager has been closed. On failure to create
// the fsnotify watcher the plugin directory remains unwatched and the
// returned error wraps the underlying fsnotify error.
//
// The ctx parameter controls the watcher lifetime: when ctx is canceled
// the watcher goroutine exits cleanly. Pass the application's long-lived
// context, not an initialization-only context.
func (m *Manager) StartWatching(ctx context.Context) error {
	m.watchMu.Lock()
	defer m.watchMu.Unlock()

	if m.closed.Load() {
		return ErrManagerClosed
	}
	if m.watching {
		return nil
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return coreerrs.WrapOperation(err, "create fsnotify watcher")
	}
	if err := watcher.Add(m.opts.dir); err != nil {
		_ = watcher.Close()
		return coreerrs.Wrapf(err, "add %q to watcher", m.opts.dir)
	}

	watchCtx, watchStop := context.WithCancel(ctx)
	done := make(chan struct{})
	m.watchCtx = watchCtx
	m.watchStop = watchStop
	m.watchDone = done
	m.watching = true

	// Capture the channels/context in the goroutine closure so a later
	// [Manager.StopWatching] that nils out the struct fields cannot race
	// with the goroutine's close(done).
	go m.watchLoop(watchCtx, watcher, done)

	m.logger.Info("started plugin watching",
		slog.String("dir", m.opts.dir),
		slog.Duration("debounce", m.opts.watchDebounce),
	)
	return nil
}

// StopWatching stops the filesystem watcher and waits for the watch goroutine
// to exit. Safe to call concurrently and idempotent: calling StopWatching on
// a manager that is not watching is a no-op.
//
// The `watching` flag is cleared atomically with the cancel handles before
// blocking on the goroutine's exit, so a concurrent [Manager.StartWatching]
// is free to start a new watcher without observing the stale "still watching"
// state of the one being stopped.
func (m *Manager) StopWatching() {
	m.watchMu.Lock()
	if !m.watching {
		m.watchMu.Unlock()
		return
	}
	stop := m.watchStop
	done := m.watchDone
	m.watching = false
	m.watchCtx = nil
	m.watchStop = nil
	m.watchDone = nil
	m.watchMu.Unlock()

	stop()
	<-done

	m.logger.Info("stopped plugin watching")
}

// watchLoop is the fsnotify event loop. It debounces rapid bursts of events
// into a single [Manager.Reload] call and logs — but does not propagate —
// any reload errors: the watcher is a best-effort background task and must
// not crash the manager when a single newly-dropped plugin fails to load.
//
// ctx, watcher, and done are captured at launch time rather than read from
// the Manager struct so that [Manager.StopWatching] can safely clear the
// struct fields without racing with this goroutine's final close(done).
//
// On exit — whether via ctx cancellation or abnormal termination (e.g.
// fsnotify closing the Events channel due to an internal error or the
// watched directory being removed) — the goroutine clears the Manager's
// watcher state if it is still the current one, so that [Manager.IsWatching]
// reports the truth and [Manager.StartWatching] can spin up a replacement.
func (m *Manager) watchLoop(ctx context.Context, watcher *fsnotify.Watcher, done chan struct{}) {
	defer close(done)
	defer m.resetWatchStateIfCurrent(done)
	defer func() { _ = watcher.Close() }()

	// Debounce state. debounceC is held nil when no timer is pending so the
	// select does not fire on an inactive channel.
	var (
		debounceTimer *time.Timer
		debounceC     <-chan time.Time
	)
	armDebounce := func() {
		if debounceTimer != nil {
			coretime.TimerStopAndDrain(debounceTimer)
			debounceTimer.Reset(m.opts.watchDebounce)
			return
		}
		debounceTimer = time.NewTimer(m.opts.watchDebounce)
		debounceC = debounceTimer.C
	}

	for {
		select {
		case <-ctx.Done():
			if debounceTimer != nil {
				coretime.TimerStopAndDrain(debounceTimer)
			}
			return

		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if !m.isRelevantEvent(event) {
				continue
			}
			m.logger.Debug("plugin file event",
				slog.String("file", event.Name),
				slog.String("op", event.Op.String()),
			)
			armDebounce()

		case <-debounceC:
			debounceTimer = nil
			debounceC = nil
			if err := m.Reload(ctx); err != nil {
				m.logger.Error("plugin reload from watcher failed",
					slog.Any("error", err),
				)
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			m.logger.Error("plugin watcher error", slog.Any("error", err))
		}
	}
}

// isRelevantEvent returns true for filesystem events that might surface a new
// loadable plugin: CREATE or WRITE on a file with a .so extension. Other ops
// (Remove, Rename, Chmod) are ignored because Go's plugin package cannot
// unload code from the process.
func (m *Manager) isRelevantEvent(event fsnotify.Event) bool {
	if event.Op&(fsnotify.Create|fsnotify.Write) == 0 {
		return false
	}
	return strings.EqualFold(filepath.Ext(event.Name), ".so")
}

// resetWatchStateIfCurrent clears the Manager's watcher state when invoked
// from a goroutine whose exit matches the currently-registered [done]
// channel. This lets the goroutine release ownership on abnormal exits
// (fsnotify Events/Errors channels closing) without stepping on a concurrent
// [Manager.StopWatching] that has already cleared the state itself.
func (m *Manager) resetWatchStateIfCurrent(done chan struct{}) {
	m.watchMu.Lock()
	defer m.watchMu.Unlock()

	// If StopWatching already cleared the state (watchDone == nil or
	// replaced by a subsequent StartWatching), there is nothing to do.
	if m.watchDone != done {
		return
	}

	m.watching = false
	m.watchCtx = nil
	m.watchStop = nil
	m.watchDone = nil
}
