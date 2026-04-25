// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package utils_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config/internal/utils"
)

func TestFindFile(t *testing.T) {
	// Create a temporary file for testing.
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "testfile.txt")
	require.NoError(t, os.WriteFile(tmpFile, []byte("test"), 0o644))

	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{"empty string", "", ""},
		{"relative path", "relative/path.txt", ""},
		{"absolute existing file", tmpFile, tmpFile},
		{"absolute nonexistent", "/nonexistent/path/file.txt", ""},
		{"absolute directory", tmpDir, tmpDir},
		{"dot relative", "./file.txt", ""},
		{"tilde path", "~/file.txt", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := utils.FindFile(tt.path)
			require.Equal(t, tt.expected, got)
		})
	}
}
