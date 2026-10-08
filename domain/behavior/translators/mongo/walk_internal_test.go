// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHasMapLayer(t *testing.T) {
	t.Parallel()

	type entry struct{ Secret string }
	deep := reflect.TypeFor[map[string]entry]()
	for range maxCollectionLayers + 1 {
		deep = reflect.SliceOf(deep)
	}

	tests := []struct {
		name string
		v    reflect.Value
		want bool
	}{
		{name: "invalid", v: reflect.Value{}},
		{name: "slice of structs", v: reflect.ValueOf([]entry{})},
		{name: "map", v: reflect.ValueOf(map[string]entry{}), want: true},
		{name: "pointer to map", v: reflect.ValueOf(&map[string]entry{}), want: true},
		{name: "map under slices", v: reflect.ValueOf([][]map[string]entry{}), want: true},
		{name: "array of slices", v: reflect.ValueOf([2][]entry{})},
		{name: "layers beyond the bound fail closed", v: reflect.Zero(deep), want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, hasMapLayer(tc.v))
		})
	}
}
