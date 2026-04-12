// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package toml_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config/loader/backend/toml"
)

func TestBackend_StructTagName(t *testing.T) {
	b := &toml.Backend{}
	require.Equal(t, "toml", b.StructTagName())
}

func TestBackend_FileExtensions(t *testing.T) {
	b := &toml.Backend{}
	got := b.FileExtensions()
	require.Equal(t, []string{"toml", "tml"}, got)
}

func TestBackend_Decode(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		target  any
		wantErr bool
		check   func(t *testing.T, target any)
	}{
		{
			name:  "valid TOML",
			input: "key = \"value\"\n[section]\nnum = 42",
			target: &struct {
				Key     string `toml:"key"`
				Section struct {
					Num int `toml:"num"`
				} `toml:"section"`
			}{},
			wantErr: false,
			check: func(t *testing.T, target any) {
				v := target.(*struct {
					Key     string `toml:"key"`
					Section struct {
						Num int `toml:"num"`
					} `toml:"section"`
				})
				require.Equal(t, "value", v.Key)
				require.Equal(t, 42, v.Section.Num)
			},
		},
		{
			name:  "empty input",
			input: "",
			target: &struct {
				Key string `toml:"key"`
			}{},
			wantErr: false,
			check: func(t *testing.T, target any) {
				v := target.(*struct {
					Key string `toml:"key"`
				})
				require.Empty(t, v.Key)
			},
		},
		{
			name:  "invalid TOML",
			input: "key = [invalid",
			target: &struct {
				Key string `toml:"key"`
			}{},
			wantErr: true,
			check:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &toml.Backend{}
			reader := strings.NewReader(tt.input)
			err := b.Decode(reader, tt.target)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, tt.target)
			}
		})
	}
}

func TestBackend_ImplementsInterface(t *testing.T) {
	// This test verifies that Backend implements the backend.Backend interface.
	// The actual compile-time check is already done via:
	// var _ backend.Backend = (*Backend)(nil)
	// in the source file, but we include this test for completeness.
	t.Log("Backend implements backend.Backend interface (verified at compile time)")
}
