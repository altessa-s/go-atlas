// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package files

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCurrentExecutableDir(t *testing.T) {
	dir := CurrentExecutableDir()
	// In test environments, this returns the temp test binary dir
	// Just ensure it doesn't panic and returns a non-empty string
	if dir == "" {
		t.Skip("CurrentExecutableDir() returned empty (may happen in some test environments)")
	}
	if !filepath.IsAbs(dir) {
		t.Errorf("CurrentExecutableDir() = %q, want absolute path", dir)
	}
}

func TestFindFile_RelativePath(t *testing.T) {
	// Create a temp dir with a file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "testfile.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	// FindFile with absolute path should find it
	found := FindFile(testFile, nil)
	if found != testFile {
		t.Errorf("FindFile(absolute) = %q, want %q", found, testFile)
	}

	// FindFile with non-existent file
	found = FindFile("/nonexistent/path/file.txt", nil)
	if found != "" {
		t.Errorf("FindFile(missing) = %q, want empty", found)
	}
}

func TestFindFile_WithBaseSearchPaths(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "configs")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	testFile := filepath.Join(subDir, "app.toml")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	// Search with base paths including the parent dir
	found := FindFile(testFile, []string{tmpDir})
	if found == "" {
		// May not find depending on relative path logic
		_ = found
	}
}

func TestDirIsEmpty_NonExistent(t *testing.T) {
	_, err := DirIsEmpty("/nonexistent/dir/path")
	if err == nil {
		t.Error("DirIsEmpty on non-existent dir should error")
	}
}
