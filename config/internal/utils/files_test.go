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

func TestResolvePath(t *testing.T) {
	t.Parallel()

	wd, err := os.Getwd()
	require.NoError(t, err)

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "testfile.txt")
	require.NoError(t, os.WriteFile(tmpFile, []byte("test"), 0o600))

	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{"empty string", "", ""},
		{"absolute existing file", tmpFile, tmpFile},
		{"absolute missing file is kept", "/nonexistent/path/file.txt", "/nonexistent/path/file.txt"},
		{"absolute directory", tmpDir, tmpDir},
		{"relative path is resolved against the working directory", "relative/path.txt", filepath.Join(wd, "relative", "path.txt")},
		{"dot relative", "./file.txt", filepath.Join(wd, "file.txt")},
		{"parent relative", "../file.txt", filepath.Join(filepath.Dir(wd), "file.txt")},
		{"existing relative file", "files.go", filepath.Join(wd, "files.go")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.expected, utils.ResolvePath(tt.path))
		})
	}
}
