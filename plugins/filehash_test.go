// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugins

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/encoding/hash"
)

// swapSameSizeKeepMtime replaces the file content with equal-length bytes and
// restores the original modification time, so size+mtime cannot reveal the
// change.
func swapSameSizeKeepMtime(t *testing.T, path string, data []byte) {
	t.Helper()

	stat, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, int64(len(data)), stat.Size())
	require.NoError(t, os.WriteFile(path, data, 0o644))
	require.NoError(t, os.Chtimes(path, time.Time{}, stat.ModTime()))
}

func TestReadAndHashFile_DigestMatchesBytesAfterSameSizeSwap(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "test.so")
	original := []byte("original-plugin!")
	swapped := []byte("swapped--plugin!")
	require.NoError(t, os.WriteFile(path, original, 0o644))

	mgr := NewManager(WithSignatureDisabled())
	_, h1, err := mgr.readAndHashFileFn(path)
	require.NoError(t, err)

	swapSameSizeKeepMtime(t, path, swapped)

	data, h2, err := mgr.readAndHashFileFn(path)
	require.NoError(t, err)
	require.Equal(t, swapped, data)
	require.Equal(t, hash.SHA256HexBytes(swapped), h2, "digest must describe the returned bytes")
	require.NotEqual(t, h1, h2)
}

func TestManager_RecheckHashAfterOpen_SameSizeSwap(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "test.so")
	require.NoError(t, os.WriteFile(path, []byte("original-plugin!"), 0o644))

	mgr := NewManager(WithSignatureDisabled())
	_, verified, err := mgr.readAndHashFileFn(path)
	require.NoError(t, err)

	swapSameSizeKeepMtime(t, path, []byte("swapped--plugin!"))

	err = mgr.recheckHashAfterOpen("test.so", path, verified)
	require.ErrorIs(t, err, ErrPluginModified)
}
