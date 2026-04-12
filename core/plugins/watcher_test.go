// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func TestManager_StartWatching_MissingDir(t *testing.T) {
	mgr := NewManager(WithDir("/nonexistent/plugin/dir/xyz"))
	t.Cleanup(func() { _ = mgr.Close() })

	err := mgr.StartWatching(t.Context())
	require.Error(t, err)
	assert.False(t, mgr.IsWatching())
}

func TestManager_StartWatching_AfterClose(t *testing.T) {
	mgr := NewManager(WithDir(t.TempDir()))
	require.NoError(t, mgr.Close())

	err := mgr.StartWatching(t.Context())
	assert.ErrorIs(t, err, ErrManagerClosed)
	assert.False(t, mgr.IsWatching())
}

func TestManager_StartWatching_Idempotent(t *testing.T) {
	mgr := NewManager(WithDir(t.TempDir()))
	t.Cleanup(func() { _ = mgr.Close() })

	require.NoError(t, mgr.StartWatching(t.Context()))
	assert.True(t, mgr.IsWatching())

	// Second call must be a no-op, not an error.
	require.NoError(t, mgr.StartWatching(t.Context()))
	assert.True(t, mgr.IsWatching())
}

func TestManager_StopWatching_NotStarted(t *testing.T) {
	mgr := NewManager(WithDir(t.TempDir()))
	t.Cleanup(func() { _ = mgr.Close() })

	// No-op, must not panic or block.
	mgr.StopWatching()
	assert.False(t, mgr.IsWatching())
}

func TestManager_StopWatching_Lifecycle(t *testing.T) {
	mgr := NewManager(WithDir(t.TempDir()))
	t.Cleanup(func() { _ = mgr.Close() })

	require.NoError(t, mgr.StartWatching(t.Context()))
	assert.True(t, mgr.IsWatching())

	mgr.StopWatching()
	assert.False(t, mgr.IsWatching())

	// Restart is allowed.
	require.NoError(t, mgr.StartWatching(t.Context()))
	assert.True(t, mgr.IsWatching())
	mgr.StopWatching()
	assert.False(t, mgr.IsWatching())
}

func TestManager_StartWatching_StopsOnCtxCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	mgr := NewManager(WithDir(t.TempDir()))
	t.Cleanup(func() { _ = mgr.Close() })

	require.NoError(t, mgr.StartWatching(ctx))
	assert.True(t, mgr.IsWatching())

	// Cancelling the parent context must drive the goroutine to exit and
	// the deferred cleanup must flip IsWatching back to false without an
	// explicit StopWatching call.
	cancel()
	testhelpers.WaitFor(t, time.Second, func() bool {
		return !mgr.IsWatching()
	}, "watcher did not stop after context cancel")
}

func TestManager_Close_StopsWatcher(t *testing.T) {
	mgr := NewManager(WithDir(t.TempDir()))

	require.NoError(t, mgr.StartWatching(t.Context()))
	assert.True(t, mgr.IsWatching())

	require.NoError(t, mgr.Close())
	assert.False(t, mgr.IsWatching())

	// Subsequent StartWatching on a closed manager must fail.
	err := mgr.StartWatching(t.Context())
	assert.ErrorIs(t, err, ErrManagerClosed)
}

// TestManager_Watcher_ReactsToNewFile exercises the fsnotify pipeline end
// to end: dropping a file with a .so extension into the watched directory
// must trigger a debounced Reload attempt. The file is intentionally
// invalid so the load ultimately fails, but the test does not depend on
// that — it installs a per-Manager watcherReloadHook that signals via a
// channel as soon as the watch goroutine has called Reload, regardless
// of outcome. This replaces the previous time.Sleep(200ms)-based
// assertion which was racy under CI load.
func TestManager_Watcher_ReactsToNewFile(t *testing.T) {
	dir := t.TempDir()

	mgr := NewManager(
		WithDir(dir),
		WithWatchDebounce(50*time.Millisecond),
	)
	t.Cleanup(func() { _ = mgr.Close() })

	// Install the test hook BEFORE starting the watcher so we cannot
	// miss an early reload.
	reloaded := make(chan error, 8)
	hook := func(err error) {
		// Non-blocking send so the watcher goroutine never blocks
		// even if the test exits early or the buffer fills.
		select {
		case reloaded <- err:
		default:
		}
	}
	mgr.watcherReloadHook.Store(&hook)

	require.NoError(t, mgr.StartWatching(t.Context()))

	// Drop a bogus .so file into the watched directory.
	path := filepath.Join(dir, "bogus.so")
	require.NoError(t, os.WriteFile(path, []byte("not a plugin"), 0o644))

	// Wait for the watcher to complete at least one Reload. The hook
	// signals after Reload returns, regardless of error.
	select {
	case <-reloaded:
		// Good — the watcher saw the file and ran Reload.
	case <-time.After(2 * time.Second):
		t.Fatal("watcher did not invoke Reload within 2s after .so drop")
	}

	assert.Equal(t, 0, mgr.Len(), "bogus .so must not load successfully")
	assert.True(t, mgr.IsWatching(), "watcher must stay active after a failed reload")
	// LastWatcherReloadErr exposes the failure for operators wiring
	// metrics or alerts; it should be non-nil after a bogus .so.
	assert.Error(t, mgr.LastWatcherReloadErr(),
		"failed reload must be observable via LastWatcherReloadErr")
}

// TestManager_Watcher_CleansUpOnCtxCancel verifies that the deferred state
// reset inside watchLoop runs when the goroutine exits because its parent
// context was canceled (not via StopWatching). Without the deferred reset,
// the Manager would be stuck reporting IsWatching() == true even though the
// goroutine has already exited, and StartWatching() would treat the stale
// flag as "already running" and silently refuse to start a replacement.
func TestManager_Watcher_CleansUpOnCtxCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	mgr := NewManager(WithDir(t.TempDir()))
	t.Cleanup(func() { _ = mgr.Close() })

	require.NoError(t, mgr.StartWatching(ctx))
	require.True(t, mgr.IsWatching())

	cancel()

	// The goroutine's deferred resetWatchStateIfCurrent must flip the flag
	// without any explicit StopWatching call.
	testhelpers.WaitFor(t, time.Second, func() bool {
		return !mgr.IsWatching()
	}, "watcher did not stop after context cancel")

	// Because the state was cleaned up, StartWatching on a fresh context
	// succeeds and installs a new watcher.
	require.NoError(t, mgr.StartWatching(t.Context()))
	assert.True(t, mgr.IsWatching())
}

// TestManager_Close_DuringWatcherReload exercises the race between Close and
// an in-flight Reload running inside the watch goroutine. The test does not
// directly synchronize with the goroutine; instead it drops a file (which
// starts a debounced reload), then immediately Closes the manager. The
// expected behavior is: the in-flight Reload either completes against the
// still-valid map or bails out with ErrManagerClosed, in either case without
// corrupting state or panicking.
func TestManager_Close_DuringWatcherReload(t *testing.T) {
	dir := t.TempDir()

	mgr := NewManager(
		WithDir(dir),
		WithWatchDebounce(10*time.Millisecond),
	)

	require.NoError(t, mgr.StartWatching(t.Context()))

	// Drop several bogus .so files to produce reload activity.
	for i := range 5 {
		path := filepath.Join(dir, fmt.Sprintf("p%d.so", i))
		require.NoError(t, os.WriteFile(path, []byte("not a plugin"), 0o644))
	}

	// Give the debouncer a chance to fire (or not); either way, Close must
	// be safe to call and must stop the watcher cleanly.
	time.Sleep(15 * time.Millisecond)
	require.NoError(t, mgr.Close())

	assert.False(t, mgr.IsWatching())
	assert.Equal(t, 0, mgr.Len(), "closed manager must expose empty plugin set")
}

// waitFor polls cond until it returns true or timeout elapses, failing the
// test if it never becomes true. Used for lifecycle assertions that depend
// on a background goroutine observing cancellation.
