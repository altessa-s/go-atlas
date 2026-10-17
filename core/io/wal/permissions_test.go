// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package wal_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/io/wal"
)

func requirePOSIX(t testing.TB) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits do not describe Windows ACLs")
	}
}

func TestWAL_PrivatePermissions(t *testing.T) {
	t.Parallel()
	requirePOSIX(t)
	dir := filepath.Join(t.TempDir(), "wal")
	w, _, err := wal.Open(dir, wal.WithMaxSegmentBytes(9))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, w.Close()) })
	for range 3 {
		_, err = w.Append([]byte("x"))
		require.NoError(t, err)
	}
	info, err := os.Stat(dir)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	files, err := filepath.Glob(filepath.Join(dir, "*.wal"))
	require.NoError(t, err)
	require.Len(t, files, 3)
	for _, f := range files {
		info, err := os.Stat(f)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
}

func TestWAL_RejectsInsecurePathsBeforeRecovery(t *testing.T) {
	t.Parallel()
	requirePOSIX(t)
	for _, tc := range []struct {
		name      string
		directory bool
		symlink   bool
	}{
		{"directory", true, false}, {"segment", false, false}, {"symlink directory", true, true}, {"symlink segment", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			require.NoError(t, os.Chmod(dir, 0o700))
			// Earlier empty segment would be removed by recovery if validation were incremental.
			first := filepath.Join(dir, "00000000000000000001.wal")
			second := filepath.Join(dir, "00000000000000000002.wal")
			require.NoError(t, os.WriteFile(first, nil, 0o600))
			require.NoError(t, os.WriteFile(second, []byte("torn record"), 0o600))
			target := second
			if tc.directory {
				target = dir
			}
			require.NoError(t, os.Chmod(target, 0o755))
			openDir := dir
			if tc.symlink {
				if tc.directory {
					openDir = filepath.Join(t.TempDir(), "link")
					require.NoError(t, os.Symlink(dir, openDir))
				} else {
					external := filepath.Join(t.TempDir(), "target")
					require.NoError(t, os.Rename(second, external))
					require.NoError(t, os.Symlink(external, second))
				}
			}
			w, _, err := wal.Open(openDir)
			if w != nil {
				require.NoError(t, w.Close())
			}
			require.ErrorIs(t, err, wal.ErrInsecurePermissions)
			data, err := os.ReadFile(second)
			require.NoError(t, err)
			require.Equal(t, []byte("torn record"), data)
			_, err = os.Stat(first)
			require.NoError(t, err)
		})
	}
}

func TestWAL_InsecurePermissionsOptOut(t *testing.T) {
	t.Parallel()
	requirePOSIX(t)
	dir := filepath.Join(t.TempDir(), "wal")
	w, _, err := wal.Open(dir, wal.WithInsecurePermissions())
	require.NoError(t, err)
	_, err = w.Append([]byte("retained"))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	// Compare with legacy creation under the same umask, without changing process-global state.
	reference := filepath.Join(t.TempDir(), "reference")
	require.NoError(t, os.Mkdir(reference, 0o755))
	expectedDir, err := os.Stat(reference)
	require.NoError(t, err)
	actualDir, err := os.Stat(dir)
	require.NoError(t, err)
	require.Equal(t, expectedDir.Mode().Perm(), actualDir.Mode().Perm())
	referenceFile := filepath.Join(reference, "segment")
	require.NoError(t, os.WriteFile(referenceFile, nil, 0o644))
	expectedFile, err := os.Stat(referenceFile)
	require.NoError(t, err)
	actualFile, err := os.Stat(filepath.Join(dir, "00000000000000000001.wal"))
	require.NoError(t, err)
	require.Equal(t, expectedFile.Mode().Perm(), actualFile.Mode().Perm())
	// Make the legacy bits explicit, independent of the process umask.
	require.NoError(t, os.Chmod(dir, 0o755))
	files, err := filepath.Glob(filepath.Join(dir, "*.wal"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.NoError(t, os.Chmod(files[0], 0o644))
	w, records, err := wal.Open(dir, wal.WithInsecurePermissions())
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, []byte("retained"), records[0].Payload)
	require.NoError(t, w.Close())
}

func TestWAL_RejectsNonRegularSegment(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o700))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "00000000000000000001.wal"), 0o700))
	_, _, err := wal.Open(dir)
	require.ErrorIs(t, err, os.ErrInvalid)
}
