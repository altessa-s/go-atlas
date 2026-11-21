// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package document_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/store/mongo/internal/document"
)

func TestStoredTagShape(t *testing.T) {
	t.Parallel()
	type Child struct{ Key string }
	type Embedded struct{ Promoted string }
	for _, key := range []string{"bson", "custom"} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			// Build the bare tag at runtime because go vet rejects it on a declared struct.
			typ := reflect.StructOf([]reflect.StructField{
				{Name: "Embedded", Type: reflect.TypeFor[Embedded](), Anonymous: true, Tag: reflect.StructTag(key + `:"renamed"`)},
				{Name: "Named", Type: reflect.TypeFor[Child](), Tag: reflect.StructTag(key + `:",inline"`)},
				{Name: "Dash", Type: reflect.TypeFor[string](), Tag: reflect.StructTag(key + `:"-,omitempty"`)},
				{Name: "Secret", Type: reflect.TypeFor[string](), Tag: "private"},
			})
			engine := document.New(document.NewParser(document.WithParserBSONTagName(key)), nil)
			for _, update := range []bool{false, true} {
				set, unset, err := engine.Convert(t.Context(), reflect.Zero(typ).Interface(), update)
				require.NoError(t, err)
				require.Equal(t, bson.M{"promoted": "", "named": Child{}, "dash": "", "secret": ""}, set)
				require.Empty(t, unset)
			}
		})
	}
}
