// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package id

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFile_GeneratesAndPersistsID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "service.id")

	p, err := NewFile(path)
	if err != nil {
		t.Fatalf("NewFile err=%v", err)
	}

	id1 := p.ID()
	if id1 == "" {
		t.Fatalf("ID empty; err=%v", p.Error())
	}

	// New provider instance should read the same value.
	p2, err := NewFile(path)
	if err != nil {
		t.Fatalf("NewFile #2 err=%v", err)
	}
	id2 := p2.ID()
	if id2 != id1 {
		t.Fatalf("id2=%q, want %q", id2, id1)
	}

	// Basic format sanity: 16 bytes -> 32 hex chars, uppercased.
	if len(id1) != 32 {
		t.Fatalf("len(id)=%d, want 32", len(id1))
	}
	if id1 != strings.ToUpper(id1) {
		t.Fatalf("id not uppercased: %q", id1)
	}
}

func TestFile_FileExistsButUnreadable_FailsFast(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod semantics differ on Windows")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "service.id")

	if err := os.WriteFile(path, []byte("ABC"), 0o000); err != nil {
		t.Fatalf("WriteFile err=%v", err)
	}
	// Best-effort: restore perms for cleanup on some platforms.
	defer func() { _ = os.Chmod(path, 0o600) }()

	p, err := NewFile(path)
	if err != nil {
		t.Fatalf("NewFile err=%v", err)
	}

	if got := p.ID(); got != "" {
		t.Fatalf("ID=%q, want empty due to unreadable file", got)
	}
	if p.Error() == nil {
		t.Fatalf("Error=nil, want non-nil due to unreadable file")
	}
}

func TestFile_PersistenceError_ReturnsIDButTracksError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod semantics differ on Windows")
	}

	// Create a read-only directory where we cannot create files
	dir := t.TempDir()
	readOnlyDir := filepath.Join(dir, "readonly")
	if err := os.Mkdir(readOnlyDir, 0o500); err != nil {
		t.Fatalf("Mkdir err=%v", err)
	}
	// Restore permissions for cleanup
	defer func() { _ = os.Chmod(readOnlyDir, 0o700) }()

	path := filepath.Join(readOnlyDir, "service.id")

	p, err := NewFile(path)
	if err != nil {
		t.Fatalf("NewFile err=%v", err)
	}

	// ID should still be generated and returned
	id := p.ID()
	if id == "" {
		t.Fatalf("ID empty, want non-empty even with persistence failure")
	}

	// No fatal initialization error
	if p.Error() != nil {
		t.Fatalf("Error=%v, want nil (persistence failure is not fatal)", p.Error())
	}

	// But persistence error should be tracked
	if p.PersistenceError() == nil {
		t.Fatalf("PersistenceError=nil, want non-nil due to read-only directory")
	}

	// Verify ID format is still valid
	if len(id) != 32 {
		t.Fatalf("len(id)=%d, want 32", len(id))
	}
	if id != strings.ToUpper(id) {
		t.Fatalf("id not uppercased: %q", id)
	}
}

func TestFile_NoPersistenceError_WhenFileWritten(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "service.id")

	p, err := NewFile(path)
	if err != nil {
		t.Fatalf("NewFile err=%v", err)
	}

	id := p.ID()
	if id == "" {
		t.Fatalf("ID empty; err=%v", p.Error())
	}

	// No errors should be present
	if p.Error() != nil {
		t.Fatalf("Error=%v, want nil", p.Error())
	}
	if p.PersistenceError() != nil {
		t.Fatalf("PersistenceError=%v, want nil", p.PersistenceError())
	}

	// Verify file was created
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("ID file was not created")
	}
}
