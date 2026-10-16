// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugins

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	goplugin "plugin"
)

// snapshotDirPattern names the private per-manager directory created under
// the snapshot base directory.
const snapshotDirPattern = "go-atlas-plugins-*"

// snapshotFilePattern names a single snapshot file inside that directory.
const snapshotFilePattern = "plugin-*.so"

// handleCache maps the SHA-256 of verified plugin bytes to the handle Go
// returned for them. Go's runtime refuses to load a second copy of a plugin
// that is already mapped ("plugin already loaded"), and every snapshot has a
// fresh path, so content already opened in this process must reuse its
// handle instead of being opened again.
//
// The cache is process-wide because the Go runtime state it mirrors is.
// Each entry is filled once; concurrent loads of the same content wait for
// the first open to finish.
type handleCache struct {
	entries sync.Map // SHA-256 → *handleEntry; populated as plugins are opened.
}

type handleEntry struct {
	open func() (*goplugin.Plugin, error)
}

// snapshotError marks a failure of the snapshot step itself (directory,
// write, read-back), as opposed to a failure of plugin.Open. The plugin
// file is not at fault, so the manager does not quarantine it.
type snapshotError struct{ err error }

func (e *snapshotError) Error() string { return e.err.Error() }
func (e *snapshotError) Unwrap() error { return e.err }

// processHandles is the process-wide [handleCache] used by every manager.
var processHandles = newHandleCache()

func newHandleCache() *handleCache { return &handleCache{} }

// get retains successful handles and retries failed opens on the next call.
// OnceValues also memoizes panics: a panicking plugin init must never leave
// waiters blocked or be executed again. Opens run synchronously; the core
// singleflight helper runs detached callbacks, which would let loading and
// snapshot cleanup outlive the caller on cancellation.
func (c *handleCache) get(fileHash string, open func() (*goplugin.Plugin, error)) (*goplugin.Plugin, error) {
	entry := &handleEntry{open: sync.OnceValues(open)}
	actual, _ := c.entries.LoadOrStore(fileHash, entry)
	entry, _ = actual.(*handleEntry) // This cache stores only *handleEntry values.
	raw, err := entry.open()
	if err != nil {
		c.entries.CompareAndDelete(fileHash, entry)
	}
	return raw, err
}

// ensureSnapshotDir returns the manager's private snapshot directory,
// creating it on first use under the configured base directory. It returns
// [ErrManagerClosed] once [Manager.Close] has run, so a closed manager never
// leaves a directory behind.
func (m *Manager) ensureSnapshotDir() (string, error) {
	m.snapMu.Lock()
	defer m.snapMu.Unlock()

	if m.closed.Load() {
		return "", ErrManagerClosed
	}
	if m.snapDir != "" {
		return m.snapDir, nil
	}

	base := m.opts.snapshotDir
	if base == "" {
		base = os.TempDir()
	}
	base, err := filepath.Abs(base)
	if err != nil {
		return "", coreerrs.Wrapf(err, "resolve plugin snapshot directory %q", base)
	}
	if err = checkSnapshotBase(base); err != nil {
		return "", err
	}

	// MkdirTemp creates a fresh, uniquely named 0700 directory, so nobody
	// else can create or replace files inside it.
	dir, err := os.MkdirTemp(base, snapshotDirPattern)
	if err != nil {
		return "", coreerrs.Wrapf(err, "create plugin snapshot directory in %q", base)
	}
	m.snapDir = dir
	return dir, nil
}

// checkSnapshotBase rejects a base directory that other users could use to
// rename or replace the private snapshot directory: one writable by group or
// others without the sticky bit.
func checkSnapshotBase(base string) error {
	info, err := os.Stat(base)
	if err != nil {
		return coreerrs.Wrapf(err, "plugin snapshot directory %q", base)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: %q is not a directory", ErrInsecureSnapshotDir, base)
	}
	if info.Mode().Perm()&0o022 != 0 && info.Mode()&os.ModeSticky == 0 {
		return fmt.Errorf("%w: %q is writable by group or others without the sticky bit (mode %s)",
			ErrInsecureSnapshotDir, base, info.Mode())
	}
	return nil
}

// currentSnapshotDir returns the private snapshot directory, or "" when it
// has not been created yet or was removed by [Manager.Close].
func (m *Manager) currentSnapshotDir() string {
	m.snapMu.Lock()
	defer m.snapMu.Unlock()
	return m.snapDir
}

// removeSnapshotDir deletes the private snapshot directory. Called by
// [Manager.Close] after the closed flag is set, so no new directory can be
// created afterwards.
func (m *Manager) removeSnapshotDir() error {
	m.snapMu.Lock()
	defer m.snapMu.Unlock()
	if m.snapActive != 0 {
		return nil // Last active open owns cleanup; Close never waits on plugin init.
	}
	dir := m.snapDir

	if dir == "" {
		return nil
	}
	if err := os.RemoveAll(dir); err != nil {
		return coreerrs.Wrapf(err, "remove plugin snapshot directory %q", dir)
	}
	m.snapDir = ""
	return nil
}

// openVerified opens the plugin whose verified bytes are data (SHA-256
// fileHash) and returns its handle.
//
// plugin.Open accepts only a path, and the file in the plugin directory can
// change after verification. The verified bytes are therefore copied into a
// new file in the manager's private snapshot directory, read back and
// compared with fileHash, and only that snapshot is opened. The snapshot is
// removed once plugin.Open returns; the mapping stays valid. Content already
// opened in this process reuses its handle (see [handleCache]).
func (m *Manager) openVerified(filename string, data []byte, fileHash string) (*goplugin.Plugin, error) {
	return m.handles.get(fileHash, func() (*goplugin.Plugin, error) {
		dir, err := m.ensureSnapshotDir()
		if err != nil {
			return nil, &snapshotError{err: err}
		}
		m.snapMu.Lock()
		if m.closed.Load() {
			m.snapMu.Unlock()
			return nil, &snapshotError{err: ErrManagerClosed}
		}
		m.snapActive++
		m.snapMu.Unlock()
		defer m.releaseSnapshot()
		snapshot, err := writeSnapshot(dir, data)
		if err != nil {
			return nil, &snapshotError{err: coreerrs.Wrap(err, "write snapshot")}
		}
		defer func() {
			if rmErr := os.Remove(snapshot); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
				m.logger.Warn("plugin snapshot cleanup failed",
					slog.String("file", filename),
					slog.String("snapshot", snapshot),
					slog.Any("error", rmErr),
				)
			}
		}()

		_, snapHash, err := m.readAndHashFileFn(snapshot)
		if err != nil {
			return nil, &snapshotError{err: coreerrs.Wrap(err, "read snapshot")}
		}
		if snapHash != fileHash {
			return nil, &snapshotError{err: ErrPluginModified}
		}
		return m.openPluginFn(snapshot)
	})
}

// releaseSnapshot lets the last in-flight open finish cleanup after Close.
func (m *Manager) releaseSnapshot() {
	m.snapMu.Lock()
	m.snapActive--
	m.snapMu.Unlock()
	if m.closed.Load() {
		if err := m.removeSnapshotDir(); err != nil {
			m.logger.Warn("plugin snapshot directory cleanup failed", slog.Any("error", err))
		}
	}
}

// writeSnapshot writes data to a new file in dir. The file is created
// exclusively (O_EXCL) with mode 0600 under a random name.
func writeSnapshot(dir string, data []byte) (string, error) {
	f, err := os.CreateTemp(dir, snapshotFilePattern)
	if err != nil {
		return "", err
	}
	path := f.Name()
	_, err = f.Write(data)
	err = errors.Join(err, f.Close())
	if err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}
