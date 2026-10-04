// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package yaml3_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config/loader/backend/yaml3"
)

func TestBackend_StructTagName(t *testing.T) {
	backend := &yaml3.Backend{}
	require.Equal(t, "yaml", backend.StructTagName())
}

func TestBackend_FileExtensions(t *testing.T) {
	backend := &yaml3.Backend{}
	got := backend.FileExtensions()
	require.Equal(t, []string{"yaml", "yml"}, got)
}

func TestBackend_Decode(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		check   func(t *testing.T, result map[string]any)
	}{
		{
			name:    "valid yaml",
			input:   "key: value\nsection:\n  num: 42",
			wantErr: false,
			check: func(t *testing.T, result map[string]any) {
				require.Equal(t, "value", result["key"])
				section, ok := result["section"].(map[string]any)
				require.True(t, ok, "section is not a map")
				require.Equal(t, 42, section["num"])
			},
		},
		{
			name:    "empty yaml",
			input:   "",
			wantErr: true, // YAML decoder returns EOF for empty input
			check:   nil,
		},
		{
			name:    "invalid yaml",
			input:   "key: value\n  invalid: [unclosed",
			wantErr: true,
			check:   nil,
		},
	}

	backend := &yaml3.Backend{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result map[string]any
			reader := strings.NewReader(tt.input)
			err := backend.Decode(reader, &result)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, result)
			}
		})
	}
}

func TestBackend_DecodeKeys(t *testing.T) {
	t.Parallel()

	keys, err := (&yaml3.Backend{}).DecodeKeys(strings.NewReader(`
base: &base
  a: 1
m:
  0x10: false
  yes: ""
  nil: null
merged:
  <<: *base
  b: 2
list:
  - x: 0
`))
	require.NoError(t, err)

	m := keys["m"].(map[string]any)
	require.Contains(t, m, "0x10")
	require.Contains(t, m, "yes")
	require.Contains(t, m, "nil")
	require.Nil(t, m["nil"])

	merged := keys["merged"].(map[string]any)
	require.Contains(t, merged, "a")
	require.Contains(t, merged, "b")

	list := keys["list"].([]any)
	require.Contains(t, list[0].(map[string]any), "x")

	empty, err := (&yaml3.Backend{}).DecodeKeys(strings.NewReader(""))
	require.NoError(t, err)
	require.Empty(t, empty)
}
