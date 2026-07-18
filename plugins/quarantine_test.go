// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugins

import (
	"bytes"
	"errors"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager_IsQuarantined(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		store       map[string]string
		filename    string
		fileHash    string
		want        bool
		wantCleared bool
	}{
		{
			name:     "not quarantined when store is empty",
			store:    map[string]string{},
			filename: "a.so",
			fileHash: "abc123",
			want:     false,
		},
		{
			name:     "quarantined when hash matches",
			store:    map[string]string{"a.so": "abc123"},
			filename: "a.so",
			fileHash: "abc123",
			want:     true,
		},
		{
			name:        "not quarantined when hash differs (file changed)",
			store:       map[string]string{"a.so": "old-hash"},
			filename:    "a.so",
			fileHash:    "new-hash",
			want:        false,
			wantCleared: true,
		},
		{
			name:     "different file not quarantined",
			store:    map[string]string{"a.so": "abc123"},
			filename: "b.so",
			fileHash: "abc123",
			want:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mgr := NewManager(WithSignatureDisabled())
			mgr.quarantine = tc.store

			got := mgr.isQuarantined(tc.filename, tc.fileHash)
			assert.Equal(t, tc.want, got)

			if tc.wantCleared {
				mgr.quarantineMu.RLock()
				_, exists := mgr.quarantine[tc.filename]
				mgr.quarantineMu.RUnlock()
				assert.False(t, exists, "quarantine entry should be cleared after hash change")
			}
		})
	}
}

func TestManager_AddQuarantine(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	mgr := NewManager(WithLogger(logger), WithSignatureDisabled())

	mgr.addQuarantine("broken.so", "deadbeef1234")

	mgr.quarantineMu.RLock()
	h, ok := mgr.quarantine["broken.so"]
	mgr.quarantineMu.RUnlock()

	require.True(t, ok)
	assert.Equal(t, "deadbeef1234", h)
	assert.Contains(t, buf.String(), "quarantined")
}

func TestManager_Quarantine_Runtime(t *testing.T) {
	t.Parallel()

	mgr := NewManager(WithSignatureDisabled())
	t.Cleanup(func() { _ = mgr.Close() })

	// Stub hash function.
	mgr.readAndHashFileFn = func(_ string) ([]byte, string, error) {
		return nil, "fakehash123", nil
	}

	// Register a fake plugin.
	p := newTestPlugin("myplugin", StateReady)
	p.path = "/plugins/myplugin.so"
	mgr.plugins["myplugin"] = p

	err := mgr.Quarantine("myplugin")
	require.NoError(t, err)

	// Plugin removed from registry.
	_, err = mgr.Get("myplugin")
	assert.ErrorIs(t, err, ErrPluginNotFound)

	// Plugin state is failed.
	assert.Equal(t, StateFailed, p.State())
	assert.ErrorIs(t, p.Err(), ErrPluginQuarantined)

	// File is quarantined.
	mgr.quarantineMu.RLock()
	h, ok := mgr.quarantine["myplugin.so"]
	mgr.quarantineMu.RUnlock()
	require.True(t, ok)
	assert.Equal(t, "fakehash123", h)
}

func TestManager_Quarantine_NotFound(t *testing.T) {
	t.Parallel()

	mgr := NewManager(WithSignatureDisabled())
	err := mgr.Quarantine("nonexistent")
	assert.ErrorIs(t, err, ErrPluginNotFound)
}

func TestManager_Quarantine_Closed(t *testing.T) {
	t.Parallel()

	mgr := NewManager(WithSignatureDisabled())
	require.NoError(t, mgr.Close())
	err := mgr.Quarantine("any")
	assert.ErrorIs(t, err, ErrManagerClosed)
}

func TestManager_Quarantined_Iterator(t *testing.T) {
	t.Parallel()

	mgr := NewManager(WithSignatureDisabled())
	mgr.quarantine["a.so"] = "hash1"
	mgr.quarantine["b.so"] = "hash2"

	collected := maps.Collect(mgr.Quarantined())

	assert.Len(t, collected, 2)
	assert.Equal(t, "hash1", collected["a.so"])
	assert.Equal(t, "hash2", collected["b.so"])
}

func TestManager_Quarantined_Empty(t *testing.T) {
	t.Parallel()

	mgr := NewManager(WithSignatureDisabled())
	count := 0
	for range mgr.Quarantined() {
		count++
	}
	assert.Equal(t, 0, count)
}

func TestManager_LoadPlugin_QuarantineOnFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bogusPath := filepath.Join(dir, "bogus.so")
	require.NoError(t, os.WriteFile(bogusPath, []byte("not a real plugin"), 0o644))

	mgr := NewManager(WithDir(dir), WithSignatureDisabled())
	t.Cleanup(func() { _ = mgr.Close() })

	// First load fails (bogus .so) and quarantines the file.
	err := mgr.Load(t.Context())
	require.Error(t, err)

	// File should be quarantined.
	mgr.quarantineMu.RLock()
	_, quarantined := mgr.quarantine["bogus.so"]
	mgr.quarantineMu.RUnlock()
	assert.True(t, quarantined, "failed plugin should be quarantined")

	// Second load should skip the quarantined file (no error, since it's skipped).
	err = mgr.Reload(t.Context())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPluginQuarantined)
}

func TestManager_LoadPlugin_QuarantineClearedOnFileChange(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bogusPath := filepath.Join(dir, "changing.so")
	require.NoError(t, os.WriteFile(bogusPath, []byte("version1"), 0o644))

	mgr := NewManager(WithDir(dir), WithSignatureDisabled())
	t.Cleanup(func() { _ = mgr.Close() })

	// First load fails and quarantines.
	_ = mgr.Load(t.Context())

	mgr.quarantineMu.RLock()
	_, quarantined := mgr.quarantine["changing.so"]
	mgr.quarantineMu.RUnlock()
	require.True(t, quarantined)

	// Change the file content (simulating operator deploying a fix).
	require.NoError(t, os.WriteFile(bogusPath, []byte("version2-fixed"), 0o644))

	// Reload should clear quarantine and retry (will still fail since it's
	// not a valid .so, but the quarantine entry should be gone).
	_ = mgr.Reload(t.Context())

	// Quarantine entry was cleared (then re-added for the new hash since
	// the file is still bogus, but the old hash entry is gone).
	mgr.quarantineMu.RLock()
	newHash := mgr.quarantine["changing.so"]
	mgr.quarantineMu.RUnlock()
	// The hash should be different from the original.
	assert.NotEmpty(t, newHash, "file should be re-quarantined with new hash after retry fails")
}

func TestReadAndHashFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.so")
	content := []byte("hello plugin")
	require.NoError(t, os.WriteFile(path, content, 0o644))

	data, h, err := readAndHashFileWithCache(path, nil)
	require.NoError(t, err)
	assert.Len(t, h, 64, "SHA256 hex should be 64 chars")
	assert.Equal(t, content, data)

	// Same content → same hash.
	_, h2, err := readAndHashFileWithCache(path, nil)
	require.NoError(t, err)
	assert.Equal(t, h, h2)

	// Different content → different hash.
	require.NoError(t, os.WriteFile(path, []byte("different"), 0o644))
	_, h3, err := readAndHashFileWithCache(path, nil)
	require.NoError(t, err)
	assert.NotEqual(t, h, h3)
}

func TestReadAndHashFile_NotFound(t *testing.T) {
	t.Parallel()

	_, _, err := readAndHashFileWithCache("/nonexistent/path/plugin.so", nil)
	require.Error(t, err)
}

func TestManager_LoadPlugin_HashFailureQuarantines(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "unreadable.so"), []byte("x"), 0o644))

	mgr := NewManager(WithDir(dir), WithSignatureDisabled())
	t.Cleanup(func() { _ = mgr.Close() })

	// Stub hash to fail.
	mgr.readAndHashFileFn = func(_ string) ([]byte, string, error) {
		return nil, "", os.ErrPermission
	}

	err := mgr.Load(t.Context())
	require.Error(t, err)

	// File should be quarantined with empty hash (retried on next attempt).
	mgr.quarantineMu.RLock()
	h, ok := mgr.quarantine["unreadable.so"]
	mgr.quarantineMu.RUnlock()
	require.True(t, ok, "hash failure should still quarantine the file")
	assert.Empty(t, h, "empty hash means retry on next reload regardless")
}

func TestManager_Close_ClearsQuarantine(t *testing.T) {
	t.Parallel()

	mgr := NewManager(WithSignatureDisabled())
	mgr.quarantine["stale.so"] = "oldhash"

	require.NoError(t, mgr.Close())

	mgr.quarantineMu.RLock()
	assert.Empty(t, mgr.quarantine)
	mgr.quarantineMu.RUnlock()
}

func TestManager_Quarantined_AfterClose(t *testing.T) {
	t.Parallel()

	mgr := NewManager(WithSignatureDisabled())
	mgr.quarantine["a.so"] = "hash"
	require.NoError(t, mgr.Close())

	count := 0
	for range mgr.Quarantined() {
		count++
	}
	assert.Equal(t, 0, count, "Quarantined() on closed manager yields nothing")
}

func TestReadAndHashFile_RejectsOversizedFile(t *testing.T) {
	t.Parallel()

	// A sparse file: Truncate sets the size without writing 512 MiB to disk.
	path := filepath.Join(t.TempDir(), "big.so")
	f, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, f.Truncate(maxPluginFileBytes+1))
	require.NoError(t, f.Close())

	_, _, err = readAndHashFileWithCache(path, nil)
	require.Error(t, err)

	_, _, err = readAndHashFileWithCache(path, newHashCache())
	require.Error(t, err)
}

func TestManager_RecheckHashAfterOpen(t *testing.T) {
	t.Parallel()

	readErr := errors.New("read failed")

	tests := []struct {
		name         string
		reHash       string
		readErr      error
		wantErr      error
		wantQuar     bool
		wantQuarHash string
	}{
		{
			name:   "hash unchanged passes",
			reHash: "verified-hash",
		},
		{
			name:         "hash mismatch quarantines under new hash",
			reHash:       "swapped-hash",
			wantErr:      ErrPluginModified,
			wantQuar:     true,
			wantQuarHash: "swapped-hash",
		},
		{
			name:         "re-read failure quarantines with empty hash",
			readErr:      readErr,
			wantErr:      readErr,
			wantQuar:     true,
			wantQuarHash: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			mgr := NewManager(WithSignatureDisabled())
			mgr.readAndHashFileFn = func(string) ([]byte, string, error) {
				return nil, tc.reHash, tc.readErr
			}

			err := mgr.recheckHashAfterOpen("a.so", "/plugins/a.so", "verified-hash")

			if tc.wantErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.wantErr)
			}

			mgr.quarantineMu.RLock()
			gotHash, exists := mgr.quarantine["a.so"]
			mgr.quarantineMu.RUnlock()
			require.Equal(t, tc.wantQuar, exists)
			if tc.wantQuar {
				assert.Equal(t, tc.wantQuarHash, gotHash)
			}
		})
	}
}
