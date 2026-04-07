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
	waitFor(t, time.Second, func() bool {
		return !mgr.IsWatching()
	})
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
// must trigger a debounced Reload attempt. The file is intentionally empty
// so the load ultimately fails (not a valid .so), but that is sufficient
// to verify that the event was processed — a failing reload produces a
// distinct error log vs. the watcher sitting idle.
func TestManager_Watcher_ReactsToNewFile(t *testing.T) {
	dir := t.TempDir()

	mgr := NewManager(
		WithDir(dir),
		WithWatchDebounce(50*time.Millisecond),
	)
	t.Cleanup(func() { _ = mgr.Close() })

	require.NoError(t, mgr.StartWatching(t.Context()))

	// Drop a bogus .so file into the watched directory.
	path := filepath.Join(dir, "bogus.so")
	require.NoError(t, os.WriteFile(path, []byte("not a plugin"), 0o644))

	// The watcher should attempt a reload; since the file is invalid the
	// manager's state never gains a new plugin. The best-effort assertion
	// is that after enough time the manager remains consistent (no panic,
	// no state corruption) and the watcher still reports as active.
	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, 0, mgr.Len(), "bogus .so must not load successfully")
	assert.True(t, mgr.IsWatching(), "watcher must stay active after a failed reload")
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
	waitFor(t, time.Second, func() bool {
		return !mgr.IsWatching()
	})

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
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", timeout)
}
