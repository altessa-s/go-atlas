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
