// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

// pinSelfInline is inlined into itself, which the codec cannot describe.
type pinSelfInline struct {
	Self *pinSelfInline `bson:",inline"`
}

func TestValidateValue_Pin(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		v    any
	}{
		{"slice element", []pinSelfInline{{}}},
		{"map value", map[string]pinSelfInline{"k": {}}},
		{"struct", pinSelfInline{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.ErrorContains(t, validateValue(reflect.ValueOf(tc.v), 0), "is inlined into itself")
		})
	}

	t.Run("depth limit", func(t *testing.T) {
		t.Parallel()
		require.ErrorContains(t, validateValue(reflect.ValueOf(1), maxValidateDepth+1), "value nests deeper than 256 levels")
	})

	t.Run("valid values", func(t *testing.T) {
		t.Parallel()
		type ok struct {
			N    int            `bson:"n"`
			B    []byte         `bson:"b"`
			M    map[string]int `bson:"m"`
			Omit *int           `bson:"omit,omitempty"`
		}
		require.NoError(t, validateValue(reflect.ValueOf([]ok{{B: []byte("x"), M: map[string]int{"a": 1}}}), 0))
	})
}
