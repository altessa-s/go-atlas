// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bsonfield_test

import (
	"reflect"
	"testing"

	"github.com/altessa-s/go-atlas/internal/bsonfield"
)

func tagged(_ reflect.Type, field reflect.StructField) bool {
	_, ok := field.Tag.Lookup("encryption")
	return ok
}

type node struct {
	Next   *node
	Secret string `encryption:"random,key"`
}

func TestInspect(t *testing.T) {
	t.Parallel()
	type recursive struct {
		Next *recursive `encryption:"random,key"`
	}
	type shared struct {
		Plain     *node
		Encrypted map[string][]node `encryption:"random,outer"`
	}
	for _, tc := range []struct {
		name string
		typ  reflect.Type
		want bsonfield.Selection
	}{
		{"plain", reflect.TypeFor[int](), bsonfield.NoFields},
		{"dynamic", reflect.TypeFor[map[string]any](), bsonfield.DynamicFields},
		{"recursive siblings", reflect.TypeFor[node](), bsonfield.SelectedFields},
		{"recursive overlap", reflect.TypeFor[recursive](), bsonfield.OverlappingFields},
		{"shared type", reflect.TypeFor[shared](), bsonfield.OverlappingFields},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := bsonfield.Inspect(tc.typ, tagged); got != tc.want {
				t.Errorf("Inspect(%s) = %v, want %v", tc.typ, got, tc.want)
			}
		})
	}
}
