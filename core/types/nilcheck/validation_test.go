// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nilcheck

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequireNotNil(t *testing.T) {
	tests := []struct {
		name        string
		value       any
		fieldName   string
		wantErr     bool
		errContains string
	}{
		{
			name:        "nil value returns error",
			value:       nil,
			fieldName:   "connection",
			wantErr:     true,
			errContains: "connection is required",
		},
		{
			name:      "non-nil value returns nil",
			value:     "some value",
			fieldName: "connection",
			wantErr:   false,
		},
		{
			name:        "nil pointer returns error",
			value:       (*string)(nil),
			fieldName:   "config",
			wantErr:     true,
			errContains: "config is required",
		},
		{
			name:        "nil slice returns error",
			value:       ([]string)(nil),
			fieldName:   "items",
			wantErr:     true,
			errContains: "items is required",
		},
		{
			name:        "nil map returns error",
			value:       (map[string]string)(nil),
			fieldName:   "mapping",
			wantErr:     true,
			errContains: "mapping is required",
		},
		{
			name:      "empty slice is not nil",
			value:     []string{},
			fieldName: "items",
			wantErr:   false,
		},
		{
			name:      "empty map is not nil",
			value:     map[string]string{},
			fieldName: "mapping",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RequireNotNil(tt.value, tt.fieldName)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestChecker(t *testing.T) {
	t.Run("no errors when all values present", func(t *testing.T) {
		nc := NewChecker("TestFactory").
			Check("value1", "dep1").
			Check("value2", "dep2")

		assert.False(t, nc.HasErrors())
		assert.NoError(t, nc.Error())
		assert.Empty(t, nc.Errors())
	})

	t.Run("errors when value is nil", func(t *testing.T) {
		nc := NewChecker("TestFactory").
			Check(nil, "dep1").
			Check("value2", "dep2")

		assert.True(t, nc.HasErrors())
		assert.Error(t, nc.Error())
		assert.Len(t, nc.Errors(), 1)
		assert.Contains(t, nc.Error().Error(), "TestFactory: dep1 is required")
	})

	t.Run("collects multiple errors", func(t *testing.T) {
		nc := NewChecker("TestFactory").
			Check(nil, "dep1").
			Check(nil, "dep2").
			Check("value3", "dep3")

		assert.True(t, nc.HasErrors())
		assert.Len(t, nc.Errors(), 2)
		// Error() returns only the first error
		assert.Contains(t, nc.Error().Error(), "dep1 is required")
	})

	t.Run("reset clears errors", func(t *testing.T) {
		nc := NewChecker("TestFactory").
			Check(nil, "dep1")

		assert.True(t, nc.HasErrors())

		nc.Reset()

		assert.False(t, nc.HasErrors())
		assert.NoError(t, nc.Error())
	})

	t.Run("handles nil interface with nil underlying value", func(t *testing.T) {
		var nilPtr *string
		nc := NewChecker("TestFactory").
			Check(nilPtr, "pointer")

		assert.True(t, nc.HasErrors())
		assert.Contains(t, nc.Error().Error(), "pointer is required")
	})
}

func TestIsNil(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  bool
	}{
		{"nil", nil, true},
		{"nil pointer", (*string)(nil), true},
		{"nil slice", ([]string)(nil), true},
		{"nil map", (map[string]string)(nil), true},
		{"nil chan", (chan int)(nil), true},
		{"nil func", (func())(nil), true},
		{"non-nil string", "hello", false},
		{"non-nil int", 42, false},
		{"empty slice", []string{}, false},
		{"empty map", map[string]string{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsNil(tt.value))
		})
	}
}
