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

	"github.com/stretchr/testify/require"
)

func TestFile_GeneratesAndPersistsID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "service.id")

	p, err := NewFile(path)
	require.NoError(t, err)

	id1 := p.ID()
	require.NotEmpty(t, id1, "ID empty; err=%v", p.Error())

	// New provider instance should read the same value.
	p2, err := NewFile(path)
	require.NoError(t, err)
	id2 := p2.ID()
	require.Equal(t, id1, id2)

	// Basic format sanity: 16 bytes -> 32 hex chars, uppercased.
	require.Len(t, id1, 32)
	require.Equal(t, strings.ToUpper(id1), id1, "id not uppercased: %q", id1)
}

func TestFile_FileExistsButUnreadable_FailsFast(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod semantics differ on Windows")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "service.id")

	require.NoError(t, os.WriteFile(path, []byte("ABC"), 0o000))
	// Best-effort: restore perms for cleanup on some platforms.
	defer func() { _ = os.Chmod(path, 0o600) }()

	p, err := NewFile(path)
	require.NoError(t, err)

	require.Empty(t, p.ID(), "ID should be empty due to unreadable file")
	require.NotNil(t, p.Error(), "Error should be non-nil due to unreadable file")
}

func TestFile_PersistenceError_ReturnsIDButTracksError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod semantics differ on Windows")
	}

	// Create a read-only directory where we cannot create files
	dir := t.TempDir()
	readOnlyDir := filepath.Join(dir, "readonly")
	require.NoError(t, os.Mkdir(readOnlyDir, 0o500))
	// Restore permissions for cleanup
	defer func() { _ = os.Chmod(readOnlyDir, 0o700) }()

	path := filepath.Join(readOnlyDir, "service.id")

	p, err := NewFile(path)
	require.NoError(t, err)

	// ID should still be generated and returned
	id := p.ID()
	require.NotEmpty(t, id, "ID should be non-empty even with persistence failure")

	// No fatal initialization error
	require.Nil(t, p.Error(), "persistence failure is not fatal")

	// But persistence error should be tracked
	require.NotNil(t, p.PersistenceError(), "PersistenceError should be non-nil due to read-only directory")

	// Verify ID format is still valid
	require.Len(t, id, 32)
	require.Equal(t, strings.ToUpper(id), id, "id not uppercased: %q", id)
}

func TestFile_NoPersistenceError_WhenFileWritten(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "service.id")

	p, err := NewFile(path)
	require.NoError(t, err)

	id := p.ID()
	require.NotEmpty(t, id, "ID empty; err=%v", p.Error())

	// No errors should be present
	require.Nil(t, p.Error())
	require.Nil(t, p.PersistenceError())

	// Verify file was created
	_, err = os.Stat(path)
	require.False(t, os.IsNotExist(err), "ID file was not created")
}
