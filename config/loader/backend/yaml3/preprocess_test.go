// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package yaml3_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config/loader/backend/yaml3"

	coreio "github.com/altessa-s/go-atlas/core/io"
)

func TestBackend_Preprocess_SimpleInclude(t *testing.T) {
	tmpDir := t.TempDir()

	subYaml := filepath.Join(tmpDir, "sub.yaml")
	subContent := "included: value\ndata: 123"
	require.NoError(t, os.WriteFile(subYaml, []byte(subContent), 0644))

	mainContent := "main: config\n!include sub.yaml\nafter: include"

	backend := &yaml3.Backend{}
	result, err := backend.Preprocess(mainContent, tmpDir, tmpDir, 0)
	require.NoError(t, err)

	require.Contains(t, result, "included: value")
	require.Contains(t, result, "data: 123")
	require.Contains(t, result, "main: config")
}

func TestBackend_Preprocess_NestedInclude(t *testing.T) {
	tmpDir := t.TempDir()

	deepYaml := filepath.Join(tmpDir, "deep.yaml")
	deepContent := "deepest: value"
	require.NoError(t, os.WriteFile(deepYaml, []byte(deepContent), 0644))

	subYaml := filepath.Join(tmpDir, "sub.yaml")
	subContent := "sub: value\n!include deep.yaml"
	require.NoError(t, os.WriteFile(subYaml, []byte(subContent), 0644))

	mainContent := "main: config\n!include sub.yaml"

	backend := &yaml3.Backend{}
	result, err := backend.Preprocess(mainContent, tmpDir, tmpDir, 0)
	require.NoError(t, err)

	require.Contains(t, result, "deepest: value")
	require.Contains(t, result, "sub: value")
}

func TestBackend_Preprocess_IndentedInclude(t *testing.T) {
	tmpDir := t.TempDir()

	subYaml := filepath.Join(tmpDir, "sub.yaml")
	subContent := "key1: value1\nkey2: value2"
	require.NoError(t, os.WriteFile(subYaml, []byte(subContent), 0644))

	mainContent := "root:\n  !include sub.yaml"

	backend := &yaml3.Backend{}
	result, err := backend.Preprocess(mainContent, tmpDir, tmpDir, 0)
	require.NoError(t, err)

	require.Contains(t, result, "  key1: value1")
	require.Contains(t, result, "  key2: value2")
}

func TestBackend_Preprocess_NoIncludes(t *testing.T) {
	tmpDir := t.TempDir()
	content := "key: value\nsection:\n  nested: data"

	backend := &yaml3.Backend{}
	result, err := backend.Preprocess(content, tmpDir, tmpDir, 0)
	require.NoError(t, err)

	// Result should have content plus trailing newlines from scanner
	require.Contains(t, result, "key: value")
	require.Contains(t, result, "nested: data")
}

func TestBackend_Preprocess_InvalidExtension(t *testing.T) {
	tmpDir := t.TempDir()

	txtFile := filepath.Join(tmpDir, "file.txt")
	require.NoError(t, os.WriteFile(txtFile, []byte("text content"), 0644))

	mainContent := "!include file.txt"

	backend := &yaml3.Backend{}
	_, err := backend.Preprocess(mainContent, tmpDir, tmpDir, 0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid extension")
}

func TestBackend_Preprocess_PathTraversal(t *testing.T) {
	tmpDir := t.TempDir()

	mainContent := "!include ../../../etc/passwd.yaml"

	backend := &yaml3.Backend{}
	_, err := backend.Preprocess(mainContent, tmpDir, tmpDir, 0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "security error")
}

func TestBackend_Preprocess_CircularInclude(t *testing.T) {
	tmpDir := t.TempDir()

	aYaml := filepath.Join(tmpDir, "a.yaml")
	bYaml := filepath.Join(tmpDir, "b.yaml")

	aContent := "a: value\n!include b.yaml"
	bContent := "b: value\n!include a.yaml"

	require.NoError(t, os.WriteFile(aYaml, []byte(aContent), 0644))
	require.NoError(t, os.WriteFile(bYaml, []byte(bContent), 0644))

	mainContent := "!include a.yaml"

	backend := &yaml3.Backend{}
	_, err := backend.Preprocess(mainContent, tmpDir, tmpDir, 0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "circular include")
}

func TestBackend_Preprocess_FileNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	mainContent := "!include nonexistent.yaml"

	backend := &yaml3.Backend{}
	_, err := backend.Preprocess(mainContent, tmpDir, tmpDir, 0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to read included file")
}

func TestBackend_Preprocess_MaxDepth(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a chain of 11 includes to exceed maxIncludeDepth of 10
	for i := range 11 {
		filename := filepath.Join(tmpDir, yamlName(i))
		var content string
		if i == 10 {
			content = "final: value"
		} else {
			content = "!include " + yamlName(i+1)
		}
		require.NoError(t, os.WriteFile(filename, []byte(content), 0644))
	}

	mainContent := "!include " + yamlName(0)

	backend := &yaml3.Backend{}
	_, err := backend.Preprocess(mainContent, tmpDir, tmpDir, 0)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "max include depth"))
}

func yamlName(i int) string {
	return "level" + string(rune('0'+i)) + ".yaml"
}

func TestBackend_Preprocess_MaxBytes(t *testing.T) {
	t.Parallel()

	const limit = 1024

	tests := []struct {
		name    string
		files   map[string]string
		main    string
		wantErr bool
	}{
		{
			name:    "include larger than the limit",
			files:   map[string]string{"big.yaml": "filler: " + strings.Repeat("a", 4*limit) + "\n"},
			main:    "main: config\n!include big.yaml\n",
			wantErr: true,
		},
		{
			name:    "small includes expanding beyond the limit",
			files:   map[string]string{"part.yaml": "filler: " + strings.Repeat("a", limit/4) + "\n"},
			main:    "a:\n  !include part.yaml\nb:\n  !include part.yaml\nc:\n  !include part.yaml\nd:\n  !include part.yaml\n",
			wantErr: true,
		},
		{
			name:  "under the limit",
			files: map[string]string{"part.yaml": "key: value\n"},
			main:  "a:\n  !include part.yaml\nb:\n  !include part.yaml\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			for name, content := range tc.files {
				require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
			}

			backend := &yaml3.Backend{}
			out, err := backend.Preprocess(tc.main, dir, dir, limit)
			if tc.wantErr {
				require.Error(t, err)
				require.True(t, errors.Is(err, coreio.ErrReadLimitExceeded), "got %v", err)
				return
			}
			require.NoError(t, err)
			require.LessOrEqual(t, len(out), limit)
		})
	}
}
