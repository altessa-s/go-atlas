// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package yaml3_test

import (
	"strings"
	"testing"

	"github.com/altessa-s/go-atlas/config/loader/backend/yaml3"
)

func TestBackend_StructTagName(t *testing.T) {
	backend := &yaml3.Backend{}
	got := backend.StructTagName()
	want := "yaml"
	if got != want {
		t.Errorf("StructTagName() = %q, want %q", got, want)
	}
}

func TestBackend_FileExtensions(t *testing.T) {
	backend := &yaml3.Backend{}
	got := backend.FileExtensions()
	want := []string{"yaml", "yml"}

	if len(got) != len(want) {
		t.Fatalf("FileExtensions() length = %d, want %d", len(got), len(want))
	}

	for i, ext := range got {
		if ext != want[i] {
			t.Errorf("FileExtensions()[%d] = %q, want %q", i, ext, want[i])
		}
	}
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
				if result["key"] != "value" {
					t.Errorf("key = %v, want %q", result["key"], "value")
				}
				section, ok := result["section"].(map[string]any)
				if !ok {
					t.Fatalf("section is not a map")
				}
				if section["num"] != 42 {
					t.Errorf("section.num = %v, want 42", section["num"])
				}
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

			if (err != nil) != tt.wantErr {
				t.Errorf("Decode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.check != nil {
				tt.check(t, result)
			}
		})
	}
}
