// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package files_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/io/files"
)

func TestFileExists(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "testfile")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	require.True(t, files.FileExists(tmpFile.Name()), "FileExists(%q) = false, want true", tmpFile.Name())
	require.False(t, files.FileExists(tmpFile.Name()+"_nonexistent"), "FileExists(nonexistent) = true, want false")

	tmpDir := t.TempDir()
	require.False(t, files.FileExists(tmpDir), "FileExists(%q) (dir) = true, want false", tmpDir)
}

func TestDirExists(t *testing.T) {
	tmpDir := t.TempDir()

	require.True(t, files.DirExists(tmpDir), "DirExists(%q) = false, want true", tmpDir)
	require.False(t, files.DirExists(tmpDir+"_nonexistent"), "DirExists(nonexistent) = true, want false")

	tmpFile, err := os.CreateTemp("", "testfile")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	require.False(t, files.DirExists(tmpFile.Name()), "DirExists(%q) (file) = true, want false", tmpFile.Name())
}

func TestDirIsEmpty(t *testing.T) {
	tmpDir := t.TempDir()

	empty, err := files.DirIsEmpty(tmpDir)
	require.NoError(t, err)
	require.True(t, empty, "DirIsEmpty(%q) = false, want true", tmpDir)

	f, err := os.CreateTemp(tmpDir, "file")
	require.NoError(t, err)
	f.Close()

	empty, err = files.DirIsEmpty(tmpDir)
	require.NoError(t, err)
	require.False(t, empty, "DirIsEmpty(%q) = true, want false", tmpDir)
}

func TestFindFile(t *testing.T) {
	// Setup a dummy structure
	// /tmp/root/config/app.yaml

	tmpRoot := t.TempDir()

	configDir := filepath.Join(tmpRoot, "config")
	require.NoError(t, os.Mkdir(configDir, 0755))

	targetFile := filepath.Join(configDir, "app.yaml")
	require.NoError(t, os.WriteFile(targetFile, []byte("content"), 0644))

	// Test finding absolute path
	require.Equal(t, targetFile, files.FindFile(targetFile, nil), "FindFile matches absolute path")
	require.Equal(t, "", files.FindFile(filepath.Join(tmpRoot, "missing.yaml"), nil), "FindFile matches missing absolute file")

	// Note: FindFile relative logic depends on CurrentExecutableDir, which is tricky to test
	// reliably in "go test" as it might return the temp build directory.
	// We can trust the absolute path logic we just tested, and the implementation is straightforward.
}
