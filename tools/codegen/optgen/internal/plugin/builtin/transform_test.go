// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package builtin

import (
	"testing"

	"github.com/altessa-s/go-atlas/tools/codegen/optgen/model"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/plugin"
)

func TestBuildStringTransformChain(t *testing.T) {
	tests := []struct {
		name              string
		varName           string
		modifiers         []string
		expectedTransform string
		expectedHasImport bool
	}{
		{
			name:              "no modifiers",
			varName:           "v",
			modifiers:         []string{},
			expectedTransform: "v",
			expectedHasImport: false,
		},
		{
			name:              "single lower",
			varName:           "v",
			modifiers:         []string{"lower"},
			expectedTransform: "strings.ToLower(v)",
			expectedHasImport: true,
		},
		{
			name:              "single upper",
			varName:           "val",
			modifiers:         []string{"upper"},
			expectedTransform: "strings.ToUpper(val)",
			expectedHasImport: true,
		},
		{
			name:              "single trimspaces",
			varName:           "v",
			modifiers:         []string{"trimspaces"},
			expectedTransform: "strings.TrimSpace(v)",
			expectedHasImport: true,
		},
		{
			name:              "multiple modifiers - lower and trimspaces",
			varName:           "v",
			modifiers:         []string{"lower", "trimspaces"},
			expectedTransform: "strings.TrimSpace(strings.ToLower(v))",
			expectedHasImport: true,
		},
		{
			name:              "multiple modifiers - upper, lower, trimspaces",
			varName:           "val",
			modifiers:         []string{"upper", "lower", "trimspaces"},
			expectedTransform: "strings.TrimSpace(strings.ToLower(strings.ToUpper(val)))",
			expectedHasImport: true,
		},
		{
			name:              "with unknown modifier mixed in",
			varName:           "v",
			modifiers:         []string{"lower", "unknown", "upper"},
			expectedTransform: "strings.ToUpper(strings.ToLower(v))",
			expectedHasImport: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := plugin.NewGenerationContext("p", "T", "Option", false, nil, nil, model.GenericInfo{})
			field := model.OptField{FieldName: "x", Type: "string"}
			transform := BuildStringTransformChain(ctx, field, tt.varName, tt.modifiers)
			if transform != tt.expectedTransform {
				t.Errorf("BuildStringTransformChain() transform = %q, want %q", transform, tt.expectedTransform)
			}

			hasStrings := false
			for _, imp := range ctx.GetAdditionalImports() {
				if imp.Path == "strings" {
					hasStrings = true
					break
				}
			}
			if hasStrings != tt.expectedHasImport {
				t.Errorf("BuildStringTransformChain() strings import = %v, want %v", hasStrings, tt.expectedHasImport)
			}
		})
	}
}

func TestHasStringModifier(t *testing.T) {
	tests := []struct {
		name      string
		modifiers []string
		expected  bool
	}{
		{
			name:      "no modifiers",
			modifiers: []string{},
			expected:  false,
		},
		{
			name:      "has lower",
			modifiers: []string{"lower"},
			expected:  true,
		},
		{
			name:      "has upper",
			modifiers: []string{"upper"},
			expected:  true,
		},
		{
			name:      "has trimspaces",
			modifiers: []string{"trimspaces"},
			expected:  true,
		},
		{
			name:      "has multiple string modifiers",
			modifiers: []string{"lower", "trimspaces"},
			expected:  true,
		},
		{
			name:      "has only non-string modifiers",
			modifiers: []string{"dedup", "unknown"},
			expected:  false,
		},
		{
			name:      "has mixed modifiers",
			modifiers: []string{"dedup", "lower", "unknown"},
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasStringModifier(tt.modifiers)
			if result != tt.expected {
				t.Errorf("HasStringModifier() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestHasModifier(t *testing.T) {
	tests := []struct {
		name      string
		modifiers []string
		modifier  string
		expected  bool
	}{
		{
			name:      "empty modifiers",
			modifiers: []string{},
			modifier:  "dedup",
			expected:  false,
		},
		{
			name:      "has modifier",
			modifiers: []string{"dedup", "lower"},
			modifier:  "dedup",
			expected:  true,
		},
		{
			name:      "does not have modifier",
			modifiers: []string{"lower", "upper"},
			modifier:  "dedup",
			expected:  false,
		},
		{
			name:      "single modifier match",
			modifiers: []string{"trimspaces"},
			modifier:  "trimspaces",
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := plugin.HasModifier(tt.modifiers, tt.modifier)
			if result != tt.expected {
				t.Errorf("HasModifier() = %v, want %v", result, tt.expected)
			}
		})
	}
}
