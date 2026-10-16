// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugins

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/encoding/hash"
	"github.com/altessa-s/go-atlas/core/runtime/concurrency"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	goplugin "plugin"
)

// errStubOpen is returned by stubbed openPluginFn implementations so the
// load stops right after the open step without a real .so file.
var errStubOpen = errors.New("stub open")

// writeSignedPlugin writes data to dir/filename together with a detached
// Ed25519 signature and returns the .so path.
func writeSignedPlugin(t testing.TB, dir, filename string, priv ed25519.PrivateKey, data []byte) string {
	t.Helper()
	path := filepath.Join(dir, filename)
	require.NoError(t, os.WriteFile(path, data, 0o644))
	digest := sha256.Sum256(data)
	require.NoError(t, os.WriteFile(path+".sig", ed25519.Sign(priv, digest[:]), 0o600))
	return path
}

// newSnapshotTestManager builds a manager with Ed25519 signature
// verification over pluginDir, a private snapshot base and an isolated
// handle cache.
func newSnapshotTestManager(t testing.TB, pluginDir string, pub ed25519.PublicKey) *Manager {
	t.Helper()
	mgr := NewManager(
		WithDir(pluginDir),
		WithSnapshotDir(t.TempDir()),
		WithSignature(SignatureOptions{Mode: SignatureRequire, PublicKey: pub}),
	)
	mgr.handles = newHandleCache()
	t.Cleanup(func() { _ = mgr.Close() })
	return mgr
}

// TestManager_Load_ExecutesVerifiedSnapshot reproduces SB-002: the .so is
// replaced after its signature was verified. The manager must hand
// plugin.Open a private snapshot holding the verified bytes, never the
// producer-controlled original.
func TestManager_Load_ExecutesVerifiedSnapshot(t *testing.T) {
	t.Parallel()

	pub, priv := testhelpers.GenerateEd25519Key(t)
	pluginDir := t.TempDir()
	signed := []byte("signed plugin bytes")
	evil := []byte("swapped evil plugin bytes")
	original := writeSignedPlugin(t, pluginDir, "a.so", priv, signed)

	mgr := newSnapshotTestManager(t, pluginDir, pub)

	var openedPath string
	mgr.openPluginFn = func(path string) (*goplugin.Plugin, error) {
		// The attacker swaps the original after verification.
		require.NoError(t, os.WriteFile(original, evil, 0o644))

		openedPath = path
		got, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, signed, got, "opened file must hold the verified bytes")

		info, err := os.Lstat(path)
		require.NoError(t, err)
		require.True(t, info.Mode().IsRegular())
		require.Zero(t, info.Mode().Perm()&0o077, "snapshot must be private")

		dirInfo, err := os.Stat(filepath.Dir(path))
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o700), dirInfo.Mode().Perm())
		return nil, errStubOpen
	}

	err := mgr.Load(t.Context())
	require.ErrorIs(t, err, errStubOpen)

	require.NotEmpty(t, openedPath)
	require.NotEqual(t, original, openedPath, "plugin.Open must not reopen the original path")
	snapDir := mgr.currentSnapshotDir()
	require.Equal(t, snapDir, filepath.Dir(openedPath))
	require.NoFileExists(t, openedPath, "snapshot must be removed after open")

	require.NoError(t, mgr.Close())
	require.NoDirExists(t, snapDir, "Close must remove the snapshot directory")
}

// TestManager_Load_SnapshotMismatchFailsClosed checks that bytes read back
// from the snapshot are compared with the verified bytes before opening.
func TestManager_Load_SnapshotMismatchFailsClosed(t *testing.T) {
	t.Parallel()

	pub, priv := testhelpers.GenerateEd25519Key(t)
	pluginDir := t.TempDir()
	original := writeSignedPlugin(t, pluginDir, "a.so", priv, []byte("signed plugin bytes"))

	mgr := newSnapshotTestManager(t, pluginDir, pub)
	mgr.readAndHashFileFn = func(path string) ([]byte, string, error) {
		data, sum, err := readAndHashFile(path)
		if path != original {
			sum = "tampered"
		}
		return data, sum, err
	}
	mgr.openPluginFn = func(string) (*goplugin.Plugin, error) {
		require.Fail(t, "openPluginFn must not run when the snapshot differs")
		return nil, errStubOpen
	}

	err := mgr.Load(t.Context())
	require.ErrorIs(t, err, ErrPluginModified)

	// The producer's content is not at fault: no quarantine entry.
	require.Empty(t, maps.Collect(mgr.Quarantined()), "snapshot mismatch must not quarantine the plugin")
}

// TestManager_Load_ReusesHandleForVerifiedContent checks that verified
// content already opened in the process is not opened a second time (Go
// rejects a second copy of the same plugin with "plugin already loaded").
func TestManager_Load_ReusesHandleForVerifiedContent(t *testing.T) {
	t.Parallel()

	pub, priv := testhelpers.GenerateEd25519Key(t)
	pluginDir := t.TempDir()
	data := []byte("signed plugin bytes")
	writeSignedPlugin(t, pluginDir, "a.so", priv, data)

	mgr := newSnapshotTestManager(t, pluginDir, pub)
	opens := 0
	mgr.openPluginFn = func(string) (*goplugin.Plugin, error) {
		opens++
		return &goplugin.Plugin{}, nil
	}

	// The zero handle exports no Descriptor, so each load fails after the
	// open step; the second attempt must reuse the cached handle.
	require.ErrorIs(t, mgr.loadPlugin(t.Context(), "a.so"), ErrNoDescriptor)
	mgr.quarantineMu.Lock()
	clear(mgr.quarantine)
	mgr.quarantineMu.Unlock()
	require.ErrorIs(t, mgr.loadPlugin(t.Context(), "a.so"), ErrNoDescriptor)
	require.Equal(t, 1, opens)
}

// TestManager_Load_InsecureSnapshotDir rejects a snapshot base directory
// that other users can write to without the sticky bit.
func TestManager_Load_InsecureSnapshotDir(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	require.NoError(t, os.Chmod(base, 0o777))

	pluginDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "a.so"), []byte("plugin"), 0o600))
	mgr := NewManager(
		WithDir(pluginDir),
		WithSnapshotDir(base),
		WithSignatureDisabled(),
	)
	t.Cleanup(func() { _ = mgr.Close() })

	require.ErrorIs(t, mgr.Load(t.Context()), ErrInsecureSnapshotDir)
}

// TestManager_Load_AfterCloseDoesNotCreateSnapshotDir checks that a closed
// manager never creates a snapshot directory.
func TestManager_Load_AfterCloseDoesNotCreateSnapshotDir(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	mgr := NewManager(WithDir(t.TempDir()), WithSnapshotDir(base), WithSignatureDisabled())
	require.NoError(t, mgr.Close())

	_, err := mgr.ensureSnapshotDir()
	require.ErrorIs(t, err, ErrManagerClosed)

	entries, err := os.ReadDir(base)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestManager_ExpandSandboxOptions_AddsSnapshotDir(t *testing.T) {
	t.Parallel()

	mgr := NewManager(WithSignatureDisabled())
	mgr.snapDir = "/var/tmp/go-atlas-plugins-x"

	got := mgr.expandSandboxOptions(SandboxOptions{
		Landlock: LandlockOptions{Enabled: true, ReadWritePaths: []string{"/data"}},
	})
	require.Equal(t, []string{"/data", "/var/tmp/go-atlas-plugins-x"}, got.Landlock.ReadWritePaths)

	off := mgr.expandSandboxOptions(SandboxOptions{})
	require.Empty(t, off.Landlock.ReadWritePaths)
}

func TestManager_SnapshotCloseDuringOpen(t *testing.T) {
	t.Parallel()
	mgr := NewManager(WithSnapshotDir(t.TempDir()), WithSignatureDisabled())
	mgr.handles = newHandleCache()
	t.Cleanup(func() { require.NoError(t, mgr.Close()) })
	var dir string
	mgr.openPluginFn = func(path string) (*goplugin.Plugin, error) {
		dir = filepath.Dir(path)
		// Reentrant Close must not deadlock or remove the file being opened.
		require.NoError(t, mgr.Close())
		require.FileExists(t, path)
		return nil, errStubOpen
	}
	data := []byte("snapshot")
	_, err := mgr.openVerified("a.so", data, hash.SHA256HexBytes(data))
	require.ErrorIs(t, err, errStubOpen)
	require.NoDirExists(t, dir)
}

func TestHandleCache(t *testing.T) {
	t.Parallel()
	t.Run("success is shared", func(t *testing.T) {
		t.Parallel()
		cache := newHandleCache()
		var calls atomic.Int32
		want := &goplugin.Plugin{}
		err := concurrency.Process(t.Context(), []int{1, 2, 3, 4}, func(_ context.Context, _ int) error {
			got, err := cache.get("hash", func() (*goplugin.Plugin, error) {
				calls.Add(1)
				return want, nil
			})
			if err != nil {
				return err
			}
			if got != want {
				return errors.New("unexpected cached handle")
			}
			return nil
		})
		require.NoError(t, err)
		require.EqualValues(t, 1, calls.Load())
	})
	t.Run("failure permits retry", func(t *testing.T) {
		t.Parallel()
		cache := newHandleCache()
		_, err := cache.get("hash", func() (*goplugin.Plugin, error) { return nil, errStubOpen })
		require.ErrorIs(t, err, errStubOpen)
		want := &goplugin.Plugin{}
		got, err := cache.get("hash", func() (*goplugin.Plugin, error) { return want, nil })
		require.NoError(t, err)
		require.Same(t, want, got)
	})
	t.Run("panic is memoized", func(t *testing.T) {
		t.Parallel()
		cache := newHandleCache()
		calls := 0
		open := func() (*goplugin.Plugin, error) { calls++; panic(errStubOpen) }
		for range 2 {
			require.PanicsWithValue(t, errStubOpen, func() { _, _ = cache.get("hash", open) })
		}
		require.Equal(t, 1, calls)
	})
}

func TestManager_SnapshotReadErrorDoesNotQuarantine(t *testing.T) {
	t.Parallel()
	pub, priv := testhelpers.GenerateEd25519Key(t)
	dir := t.TempDir()
	original := writeSignedPlugin(t, dir, "a.so", priv, []byte("signed"))
	mgr := newSnapshotTestManager(t, dir, pub)
	mgr.readAndHashFileFn = func(path string) ([]byte, string, error) {
		if path != original {
			return nil, "", os.ErrPermission
		}
		return readAndHashFile(path)
	}
	mgr.openPluginFn = func(string) (*goplugin.Plugin, error) {
		require.Fail(t, "must not open unreadable snapshot")
		return nil, errStubOpen
	}
	require.ErrorIs(t, mgr.Load(t.Context()), os.ErrPermission)
	require.Empty(t, maps.Collect(mgr.Quarantined()))
	entries, err := os.ReadDir(mgr.currentSnapshotDir())
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestManager_CachedHandleStillRequiresSignature(t *testing.T) {
	t.Parallel()
	pub, priv := testhelpers.GenerateEd25519Key(t)
	dir := t.TempDir()
	original := writeSignedPlugin(t, dir, "a.so", priv, []byte("signed"))
	mgr := newSnapshotTestManager(t, dir, pub)
	opens := 0
	mgr.openPluginFn = func(string) (*goplugin.Plugin, error) { opens++; return &goplugin.Plugin{}, nil }
	require.ErrorIs(t, mgr.Load(t.Context()), ErrNoDescriptor)
	clear(mgr.quarantine)
	require.NoError(t, os.Remove(original+".sig"))
	require.ErrorIs(t, mgr.Load(t.Context()), ErrSignatureMissing)
	require.Equal(t, 1, opens)
}

func TestCheckSnapshotBase(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		mode    os.FileMode
		wantErr bool
	}{
		{"private", 0o700, false},
		{"shared nonsticky", 0o777, true},
		{"group writable", 0o770, true},
		{"shared sticky", 0o777 | os.ModeSticky, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			require.NoError(t, os.Chmod(dir, tc.mode))
			err := checkSnapshotBase(dir)
			if tc.wantErr {
				require.ErrorIs(t, err, ErrInsecureSnapshotDir)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestManager_SnapshotCloseRetriesCleanup(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory write permissions")
	}
	base := t.TempDir()
	mgr := NewManager(WithSnapshotDir(base))
	dir, err := mgr.ensureSnapshotDir()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.Chmod(base, 0o700)); require.NoError(t, mgr.Close()) })
	require.NoError(t, os.Chmod(base, 0o500))
	require.ErrorIs(t, mgr.Close(), os.ErrPermission)
	require.Equal(t, dir, mgr.currentSnapshotDir(), "failed cleanup must retain its path")
	require.NoError(t, os.Chmod(base, 0o700))
	require.NoError(t, mgr.Close())
	require.NoDirExists(t, dir)
}

func TestManager_SnapshotCloseWhileOpenBlocked(t *testing.T) {
	t.Parallel()
	mgr := NewManager(WithSnapshotDir(t.TempDir()), WithSignatureDisabled())
	mgr.handles = newHandleCache()
	entered := make(chan string, 1)
	release := make(chan struct{})
	done := make(chan error, 1)
	mgr.openPluginFn = func(path string) (*goplugin.Plugin, error) {
		entered <- path
		<-release
		return nil, errStubOpen
	}
	data := []byte("snapshot")
	go func() {
		_, err := mgr.openVerified("a.so", data, hash.SHA256HexBytes(data))
		done <- err
	}()
	// Always release the stub, including on an assertion failure.
	t.Cleanup(func() { close(release); <-done; require.NoError(t, mgr.Close()) })
	path := <-entered
	require.NoError(t, mgr.Close())
	require.FileExists(t, path, "Close must preserve an in-flight snapshot")
}

func TestManager_SnapshotPanicCleanup(t *testing.T) {
	t.Parallel()
	mgr := NewManager(WithSnapshotDir(t.TempDir()), WithSignatureDisabled())
	mgr.handles = newHandleCache()
	t.Cleanup(func() { require.NoError(t, mgr.Close()) })
	var path string
	mgr.openPluginFn = func(p string) (*goplugin.Plugin, error) { path = p; panic(errStubOpen) }
	data := []byte("snapshot")
	require.PanicsWithValue(t, errStubOpen, func() { _, _ = mgr.openVerified("a.so", data, hash.SHA256HexBytes(data)) })
	require.NoFileExists(t, path)
	require.NoError(t, mgr.Close())
	require.NoDirExists(t, filepath.Dir(path))
}

func TestHandleCache_StaleFailurePreservesReplacement(t *testing.T) {
	t.Parallel()
	cache := newHandleCache()
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	var calls atomic.Int32
	// Hold one failed waiter after the shared open completes, while another
	// failed waiter evicts the entry and a new attempt succeeds.
	old := &handleEntry{open: func() (*goplugin.Plugin, error) {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
		return nil, errStubOpen
	}}
	cache.entries.Store("hash", old)
	open := func() (*goplugin.Plugin, error) { return &goplugin.Plugin{}, nil }
	go func() { _, err := cache.get("hash", open); done <- err }()
	<-entered
	_, err := cache.get("hash", open)
	require.ErrorIs(t, err, errStubOpen)
	want, err := cache.get("hash", open)
	require.NoError(t, err)
	close(release)
	require.ErrorIs(t, <-done, errStubOpen)
	got, err := cache.get("hash", func() (*goplugin.Plugin, error) { return nil, errStubOpen })
	require.NoError(t, err, "stale failed waiter must not evict the newer successful entry")
	require.Same(t, want, got)
}

func TestManager_ConcurrentSnapshotsShareHandle(t *testing.T) {
	t.Parallel()
	pub, priv := testhelpers.GenerateEd25519Key(t)
	dir := t.TempDir()
	writeSignedPlugin(t, dir, "a.so", priv, []byte("shared signed plugin"))
	first := newSnapshotTestManager(t, dir, pub)
	second := newSnapshotTestManager(t, dir, pub)
	second.handles = first.handles
	entered := make(chan struct{})
	release := make(chan struct{})
	secondRead := make(chan struct{})
	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)
	var calls atomic.Int32
	first.openPluginFn = func(string) (*goplugin.Plugin, error) {
		calls.Add(1)
		close(entered)
		<-release
		return &goplugin.Plugin{}, nil
	}
	second.openPluginFn = func(string) (*goplugin.Plugin, error) { calls.Add(1); return &goplugin.Plugin{}, nil }
	second.readAndHashFileFn = func(path string) ([]byte, string, error) {
		close(secondRead)
		return readAndHashFile(path)
	}
	go func() { firstDone <- first.Load(t.Context()) }()
	<-entered
	go func() { secondDone <- second.Load(t.Context()) }()
	<-secondRead
	close(release)
	require.ErrorIs(t, <-firstDone, ErrNoDescriptor)
	require.ErrorIs(t, <-secondDone, ErrNoDescriptor)
	require.EqualValues(t, 1, calls.Load())
}

func TestHandleCache_ConcurrentPanicReplay(t *testing.T) {
	t.Parallel()
	cache := newHandleCache()
	started := make(chan struct{})
	var callers, opens atomic.Int32
	err := concurrency.Process(t.Context(), []int{1, 2}, func(_ context.Context, _ int) (err error) {
		if callers.Add(1) == 2 {
			close(started)
		}
		defer func() {
			if got := recover(); got != errStubOpen {
				err = errors.New("expected cached init panic")
			}
		}()
		_, err = cache.get("hash", func() (*goplugin.Plugin, error) {
			opens.Add(1)
			<-started
			panic(errStubOpen)
		})
		return err
	}, concurrency.WithConcurrency[int](2))
	require.NoError(t, err)
	require.EqualValues(t, 1, opens.Load())
}
