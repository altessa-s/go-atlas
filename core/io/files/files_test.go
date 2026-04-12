// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package files_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/altessa-s/go-atlas/core/io/files"
)

func TestFileExists(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "testfile")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	if !files.FileExists(tmpFile.Name()) {
		t.Errorf("FileExists(%q) = false, want true", tmpFile.Name())
	}

	if files.FileExists(tmpFile.Name() + "_nonexistent") {
		t.Errorf("FileExists(nonexistent) = true, want false")
	}

	tmpDir := t.TempDir()

	if files.FileExists(tmpDir) {
		t.Errorf("FileExists(%q) (dir) = true, want false", tmpDir)
	}
}

func TestDirExists(t *testing.T) {
	tmpDir := t.TempDir()

	if !files.DirExists(tmpDir) {
		t.Errorf("DirExists(%q) = false, want true", tmpDir)
	}

	if files.DirExists(tmpDir + "_nonexistent") {
		t.Errorf("DirExists(nonexistent) = true, want false")
	}

	tmpFile, err := os.CreateTemp("", "testfile")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	if files.DirExists(tmpFile.Name()) {
		t.Errorf("DirExists(%q) (file) = true, want false", tmpFile.Name())
	}
}

func TestDirIsEmpty(t *testing.T) {
	tmpDir := t.TempDir()

	empty, err := files.DirIsEmpty(tmpDir)
	if err != nil {
		t.Errorf("DirIsEmpty error = %v", err)
	}
	if !empty {
		t.Errorf("DirIsEmpty(%q) = false, want true", tmpDir)
	}

	f, err := os.CreateTemp(tmpDir, "file")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	empty, err = files.DirIsEmpty(tmpDir)
	if err != nil {
		t.Errorf("DirIsEmpty error = %v", err)
	}
	if empty {
		t.Errorf("DirIsEmpty(%q) = true, want false", tmpDir)
	}
}

func TestFindFile(t *testing.T) {
	// Setup a dummy structure
	// /tmp/root/config/app.yaml

	tmpRoot := t.TempDir()

	configDir := filepath.Join(tmpRoot, "config")
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	targetFile := filepath.Join(configDir, "app.yaml")
	if err := os.WriteFile(targetFile, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Test finding absolute path
	if found := files.FindFile(targetFile, nil); found != targetFile {
		t.Errorf("FindFile matches absolute path: got %q, want %q", found, targetFile)
	}

	if found := files.FindFile(filepath.Join(tmpRoot, "missing.yaml"), nil); found != "" {
		t.Errorf("FindFile matches missing absolute file: %q", found)
	}

	// Note: FindFile relative logic depends on CurrentExecutableDir, which is tricky to test
	// reliably in "go test" as it might return the temp build directory.
	// We can trust the absolute path logic we just tested, and the implementation is straightforward.
}
