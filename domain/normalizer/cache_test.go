// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package normalizer

import (
	"reflect"
	"testing"
)

type cacheTestStruct struct {
	Name  string `normalize:"trim,lowercase"`
	Email string `normalize:"trim"`
	Age   int
}

func TestBuildStructFieldCache(t *testing.T) {
	cache := BuildStructFieldCache(reflect.TypeFor[cacheTestStruct]())
	if cache == nil {
		t.Fatal("expected non-nil cache")
	}
	if len(cache.Fields) == 0 {
		t.Fatal("expected fields")
	}

	// Check that non-struct returns nil
	if c := BuildStructFieldCache(reflect.TypeFor[string]()); c != nil {
		t.Fatal("expected nil for non-struct type")
	}
}

func TestGetSetStructFieldCache(t *testing.T) {
	ClearStructFieldCache()

	typ := reflect.TypeFor[cacheTestStruct]()
	if got := GetStructFieldCache(typ); got != nil {
		t.Fatal("expected nil before set")
	}

	cache := BuildStructFieldCache(typ)
	SetStructFieldCache(typ, cache)

	got := GetStructFieldCache(typ)
	if got == nil {
		t.Fatal("expected non-nil after set")
	}
	if len(got.Fields) != len(cache.Fields) {
		t.Fatalf("field count mismatch: %d vs %d", len(got.Fields), len(cache.Fields))
	}
}

func TestClearStructFieldCache(t *testing.T) {
	typ := reflect.TypeFor[cacheTestStruct]()
	SetStructFieldCache(typ, BuildStructFieldCache(typ))

	ClearStructFieldCache()

	if got := GetStructFieldCache(typ); got != nil {
		t.Fatal("expected nil after clear")
	}
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
	if nameField == nil {
		t.Fatal("Name field not found")
	}
	if len(nameField.ParsedModifiers) != 2 {
		t.Fatalf("expected 2 parsed modifiers, got %d", len(nameField.ParsedModifiers))
	}
}
