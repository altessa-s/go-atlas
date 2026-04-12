// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package normalizer

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

type cacheTestStruct struct {
	Name  string `normalize:"trim,lowercase"`
	Email string `normalize:"trim"`
	Age   int
}

func TestBuildStructFieldCache(t *testing.T) {
	cache := BuildStructFieldCache(reflect.TypeFor[cacheTestStruct]())
	require.NotNil(t, cache)
	require.NotEmpty(t, cache.Fields)

	// Check that non-struct returns nil
	require.Nil(t, BuildStructFieldCache(reflect.TypeFor[string]()))
}

func TestGetSetStructFieldCache(t *testing.T) {
	ClearStructFieldCache()

	typ := reflect.TypeFor[cacheTestStruct]()
	require.Nil(t, GetStructFieldCache(typ))

	cache := BuildStructFieldCache(typ)
	SetStructFieldCache(typ, cache)

	got := GetStructFieldCache(typ)
	require.NotNil(t, got)
	require.Len(t, got.Fields, len(cache.Fields))
}

func TestClearStructFieldCache(t *testing.T) {
	typ := reflect.TypeFor[cacheTestStruct]()
	SetStructFieldCache(typ, BuildStructFieldCache(typ))

	ClearStructFieldCache()

	require.Nil(t, GetStructFieldCache(typ))
}

func TestBuildStructFieldCache_ParsedModifiers(t *testing.T) {
	cache := BuildStructFieldCache(reflect.TypeFor[cacheTestStruct]())
	// Name field has "trim,lowercase" tag
	var nameField *FieldInfo
	for i := range cache.Fields {
		if cache.Fields[i].Name == "Name" {
			nameField = &cache.Fields[i]
			break
		}
	}
	require.NotNil(t, nameField, "Name field not found")
	require.Len(t, nameField.ParsedModifiers, 2)
}
