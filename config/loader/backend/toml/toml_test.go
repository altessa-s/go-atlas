// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package toml_test

import (
	"strings"
	"testing"

	"github.com/altessa-s/go-atlas/config/loader/backend/toml"
)

func TestBackend_StructTagName(t *testing.T) {
	b := &toml.Backend{}
	got := b.StructTagName()
	want := "toml"
	if got != want {
		t.Errorf("StructTagName() = %q, want %q", got, want)
	}
}

func TestBackend_FileExtensions(t *testing.T) {
	b := &toml.Backend{}
	got := b.FileExtensions()
	want := []string{"toml", "tml"}

	if len(got) != len(want) {
		t.Fatalf("FileExtensions() returned %d extensions, want %d", len(got), len(want))
	}

	for i := range want {
		if got[i] != want[i] {
			t.Errorf("FileExtensions()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
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
				if v.Key != "value" {
					t.Errorf("Key = %q, want %q", v.Key, "value")
				}
				if v.Section.Num != 42 {
					t.Errorf("Section.Num = %d, want %d", v.Section.Num, 42)
				}
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
				if v.Key != "" {
					t.Errorf("Key = %q, want empty string", v.Key)
				}
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

			if (err != nil) != tt.wantErr {
				t.Errorf("Decode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.check != nil {
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
