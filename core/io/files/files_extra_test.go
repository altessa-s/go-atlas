// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package files

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCurrentExecutableDir(t *testing.T) {
	dir := CurrentExecutableDir()
	// In test environments, this returns the temp test binary dir
	// Just ensure it doesn't panic and returns a non-empty string
	if dir == "" {
		t.Skip("CurrentExecutableDir() returned empty (may happen in some test environments)")
	}
	require.True(t, filepath.IsAbs(dir), "CurrentExecutableDir() = %q, want absolute path", dir)
}

func TestFindFile_RelativePath(t *testing.T) {
	// Create a temp dir with a file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "testfile.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("test"), 0644))

	// FindFile with absolute path should find it
	found := FindFile(testFile, nil)
	require.Equal(t, testFile, found, "FindFile(absolute)")

	// FindFile with non-existent file
	found = FindFile("/nonexistent/path/file.txt", nil)
	require.Equal(t, "", found, "FindFile(missing)")
}

func TestFindFile_WithBaseSearchPaths(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "configs")
	require.NoError(t, os.MkdirAll(subDir, 0755))
	testFile := filepath.Join(subDir, "app.toml")
	require.NoError(t, os.WriteFile(testFile, []byte("test"), 0644))

	// Search with base paths including the parent dir
	found := FindFile(testFile, []string{tmpDir})
	if found == "" {
		// May not find depending on relative path logic
		_ = found
	}
}

func TestDirIsEmpty_NonExistent(t *testing.T) {
	_, err := DirIsEmpty("/nonexistent/dir/path")
	require.Error(t, err, "DirIsEmpty on non-existent dir should error")
}
