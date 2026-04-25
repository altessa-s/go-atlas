// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugin

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/tools/codegen/optgen/model"
)

// mockTransformPlugin is a test transform plugin.
type mockTransformPlugin struct {
	key             string
	priority        int
	applicableTypes []string
	defaultForTypes []string
	disabledBy      string
}

func (m *mockTransformPlugin) Meta() Meta {
	return Meta{
		Kind:            KindTransform,
		ApplicableTypes: m.applicableTypes,
		Priority:        m.priority,
		DefaultForTypes: m.defaultForTypes,
		DisabledBy:      m.disabledBy,
	}
}

func (m *mockTransformPlugin) Key() string { return m.key }

func (m *mockTransformPlugin) Apply(_ GenerationContext, _ model.OptField, _, expr string) (string, bool) {
	return expr, true
}

func TestGetDefaultModifiersForType(t *testing.T) {
	tests := []struct {
		name         string
		plugins      []*mockTransformPlugin
		typeStr      string
		existingMods []string
		expected     []string
	}{
		{
			name:         "no plugins",
			plugins:      nil,
			typeStr:      "string",
			existingMods: nil,
			expected:     nil,
		},
		{
			name: "plugin with DefaultForTypes matching",
			plugins: []*mockTransformPlugin{
				{
					key:             "trimspaces",
					priority:        10,
					applicableTypes: []string{"string"},
					defaultForTypes: []string{"string"},
					disabledBy:      "notrim",
				},
			},
			typeStr:      "string",
			existingMods: nil,
			expected:     []string{"trimspaces"},
		},
		{
			name: "plugin with DefaultForTypes not matching",
			plugins: []*mockTransformPlugin{
				{
					key:             "trimspaces",
					priority:        10,
					applicableTypes: []string{"string"},
					defaultForTypes: []string{"string"},
					disabledBy:      "notrim",
				},
			},
			typeStr:      "int",
			existingMods: nil,
			expected:     nil,
		},
		{
			name: "plugin disabled by disabler modifier",
			plugins: []*mockTransformPlugin{
				{
					key:             "trimspaces",
					priority:        10,
					applicableTypes: []string{"string"},
					defaultForTypes: []string{"string"},
					disabledBy:      "notrim",
				},
			},
			typeStr:      "string",
			existingMods: []string{"notrim"},
			expected:     nil,
		},
		{
			name: "plugin already explicitly specified",
			plugins: []*mockTransformPlugin{
				{
					key:             "trimspaces",
					priority:        10,
					applicableTypes: []string{"string"},
					defaultForTypes: []string{"string"},
					disabledBy:      "notrim",
				},
			},
			typeStr:      "string",
			existingMods: []string{"trimspaces"},
			expected:     nil,
		},
		{
			name: "multiple plugins sorted by priority",
			plugins: []*mockTransformPlugin{
				{
					key:             "lower",
					priority:        5,
					applicableTypes: []string{"string"},
					defaultForTypes: []string{"string"},
				},
				{
					key:             "trimspaces",
					priority:        10,
					applicableTypes: []string{"string"},
					defaultForTypes: []string{"string"},
				},
			},
			typeStr:      "string",
			existingMods: nil,
			expected:     []string{"trimspaces", "lower"}, // higher priority first
		},
		{
			name: "plugin with notrim disables trimspaces but not lower",
			plugins: []*mockTransformPlugin{
				{
					key:             "lower",
					priority:        5,
					applicableTypes: []string{"string"},
					defaultForTypes: []string{"string"},
				},
				{
					key:             "trimspaces",
					priority:        10,
					applicableTypes: []string{"string"},
					defaultForTypes: []string{"string"},
					disabledBy:      "notrim",
				},
			},
			typeStr:      "string",
			existingMods: []string{"notrim"},
			expected:     []string{"lower"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRegistry()
			for _, p := range tt.plugins {
				r.RegisterTransformPlugins(p)
			}

			got := r.GetDefaultModifiersForType(tt.typeStr, tt.existingMods)
			// Handle nil vs empty slice comparison
			if len(got) == 0 && len(tt.expected) == 0 {
				return
			}
			require.True(t, reflect.DeepEqual(got, tt.expected), "GetDefaultModifiersForType() = %v, want %v", got, tt.expected)
		})
	}
}

func TestGetAllDisablers(t *testing.T) {
	tests := []struct {
		name     string
		plugins  []*mockTransformPlugin
		expected []string
	}{
		{
			name:     "no plugins",
			plugins:  nil,
			expected: nil,
		},
		{
			name: "plugin without disabler",
			plugins: []*mockTransformPlugin{
				{
					key:             "lower",
					applicableTypes: []string{"string"},
				},
			},
			expected: nil,
		},
		{
			name: "plugin with disabler",
			plugins: []*mockTransformPlugin{
				{
					key:             "trimspaces",
					applicableTypes: []string{"string"},
					disabledBy:      "notrim",
				},
			},
			expected: []string{"notrim"},
		},
		{
			name: "multiple plugins with disablers",
			plugins: []*mockTransformPlugin{
				{
					key:             "trimspaces",
					applicableTypes: []string{"string"},
					disabledBy:      "notrim",
				},
				{
					key:             "lower",
					applicableTypes: []string{"string"},
					disabledBy:      "nolower",
				},
			},
			expected: []string{"nolower", "notrim"}, // sorted
		},
		{
			name: "duplicate disablers",
			plugins: []*mockTransformPlugin{
				{
					key:             "trimspaces",
					applicableTypes: []string{"string"},
					disabledBy:      "raw",
				},
				{
					key:             "lower",
					applicableTypes: []string{"string"},
					disabledBy:      "raw",
				},
			},
			expected: []string{"raw"}, // deduplicated
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRegistry()
			for _, p := range tt.plugins {
				r.RegisterTransformPlugins(p)
			}

			got := r.GetAllDisablers()
			// Handle nil vs empty slice comparison
			if len(got) == 0 && len(tt.expected) == 0 {
				return
			}
			require.True(t, reflect.DeepEqual(got, tt.expected), "GetAllDisablers() = %v, want %v", got, tt.expected)
		})
	}
}
