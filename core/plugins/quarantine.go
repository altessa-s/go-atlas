// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugins

import (
	"iter"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/altessa-s/go-atlas/core/encoding/hash"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// hashPrefixLen is the number of hex characters included in log messages
// and error strings when displaying a file hash. 12 hex chars = 6 bytes
// of the SHA256, enough to identify a file without cluttering output.
const hashPrefixLen = 12

// readAndHashFile reads the file at path into memory and returns the raw
// bytes together with the SHA256 hex digest. Returning the bytes avoids
// a second read when signature verification needs the same data.
//
// The entire file is read at once; acceptable for .so files (typically
// under 100 MB) at load time.
//
// Tests override this via the [Manager.readAndHashFileFn] field.
func readAndHashFile(path string) ([]byte, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	return data, hash.SHA256HexBytes(data), nil
}

// isQuarantined reports whether filename is blacklisted with the given
// hash. If the file was quarantined with a different hash (the file
// changed on disk), the quarantine entry is cleared and false is
// returned — the plugin should be retried.
func (m *Manager) isQuarantined(filename, fileHash string) bool {
	m.quarantineMu.RLock()
	storedHash, exists := m.quarantine[filename]
	m.quarantineMu.RUnlock()

	if !exists {
		return false
	}
	if storedHash == fileHash {
		return true
	}

	// File changed — clear quarantine and retry.
	m.quarantineMu.Lock()
	// Re-check under write lock; another goroutine may have cleared it.
	if h, ok := m.quarantine[filename]; ok && h != fileHash {
		delete(m.quarantine, filename)
		m.logger.Info("plugin file changed, quarantine cleared",
			slog.String("file", filename),
		)
	}
	m.quarantineMu.Unlock()

	return false
}

// addQuarantine records a file hash in the quarantine store. Subsequent
// loads of the same file (same hash) will be skipped with
// [ErrPluginQuarantined]. The entry is automatically cleared when the
// file hash changes — see [Manager.isQuarantined].
func (m *Manager) addQuarantine(filename, fileHash string) {
	m.quarantineMu.Lock()
	m.quarantine[filename] = fileHash
	m.quarantineMu.Unlock()

	m.logger.Warn("plugin quarantined; will retry when file changes",
		slog.String("file", filename),
		slog.String("hash", fileHash[:min(hashPrefixLen, len(fileHash))]),
	)
}

// Quarantine explicitly blacklists a loaded plugin by name. Use this
// when a service observes a runtime failure (panic, repeated errors)
// from a plugin's exported symbols and wants to prevent the same .so
// from being loaded again on the next [Manager.Reload].
//
// The plugin transitions to [StateFailed], is removed from the
// registry, and its file hash is recorded in the quarantine store.
// On the next Reload, the file is skipped unless its SHA256 has
// changed (i.e. the operator deployed a fix).
//
// Returns [ErrPluginNotFound] if no plugin is registered under name.
// Returns [ErrManagerClosed] if the manager has been closed.
func (m *Manager) Quarantine(name string) error {
	if m.closed.Load() {
		return ErrManagerClosed
	}

	m.mu.Lock()
	p, exists := m.plugins[name]
	if !exists {
		m.mu.Unlock()
		return ErrPluginNotFound
	}
	delete(m.plugins, name)
	m.mu.Unlock()

	_, fileHash, err := m.readAndHashFileFn(p.Path())
	if err != nil {
		// Can't hash — still quarantine by path with empty hash.
		// The empty hash won't match any future file read, so the
		// plugin will be retried on the next Reload regardless.
		m.logger.Warn("quarantine: cannot hash plugin file; quarantine is best-effort",
			slog.String("plugin", name),
			slog.Any("error", err),
		)
		fileHash = ""
	}

	m.addQuarantine(filepath.Base(p.Path()), fileHash)
	p.setErr(coreerrs.Wrapf(ErrPluginQuarantined, "plugin %q quarantined by host", name))
	p.setState(StateFailed)

	m.logger.Info("plugin quarantined by host",
		slog.String("plugin", name),
		slog.String("file", filepath.Base(p.Path())),
	)

	return nil
}

// Quarantined returns an iterator over all quarantined files
// (filename → SHA256 hash). Use this for operational visibility
// into which plugins are currently blacklisted and why a Reload
// is not picking them up.
func (m *Manager) Quarantined() iter.Seq2[string, string] {
	if m.closed.Load() {
		return func(func(string, string) bool) {}
	}

	// Snapshot under lock to avoid holding the lock during iteration.
	m.quarantineMu.RLock()
	entries := make([]quarantineEntry, 0, len(m.quarantine))
	for filename, h := range m.quarantine {
		entries = append(entries, quarantineEntry{filename: filename, hash: h})
	}
	m.quarantineMu.RUnlock()

	return func(yield func(string, string) bool) {
		for _, e := range entries {
			if !yield(e.filename, e.hash) {
				return
			}
		}
	}
}

// quarantineEntry is used by [Manager.Quarantined] to snapshot the
// quarantine map under the lock and then iterate outside it.
type quarantineEntry struct {
	filename string
	hash     string
}
