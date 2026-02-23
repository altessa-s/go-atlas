// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package reflect_test

import (
	"reflect"
	"testing"

	reflectutils "github.com/altessa-s/go-atlas/domain/converter/internal/reflect"
)

type testStruct struct{}

func TestIndirectType(t *testing.T) {
	tests := []struct {
		name string
		typ  reflect.Type
		want reflect.Type
	}{
		{"non-pointer", reflect.TypeFor[testStruct](), reflect.TypeFor[testStruct]()},
		{"single pointer", reflect.TypeFor[*testStruct](), reflect.TypeFor[testStruct]()},
		{"double pointer", reflect.TypeFor[**testStruct](), reflect.TypeFor[testStruct]()},
		{"int", reflect.TypeFor[int](), reflect.TypeFor[int]()},
		{"*int", reflect.TypeFor[*int](), reflect.TypeFor[int]()},
		{"string", reflect.TypeFor[string](), reflect.TypeFor[string]()},
		{"*string", reflect.TypeFor[*string](), reflect.TypeFor[string]()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reflectutils.IndirectType(tt.typ)
			if got != tt.want {
				t.Errorf("IndirectType(%v) = %v, want %v", tt.typ, got, tt.want)
			}
		})
	}
}

func TestMakeDst_Pointer(t *testing.T) {
	var dst *testStruct
	v := reflect.ValueOf(&dst).Elem()
	typ := reflect.TypeFor[testStruct]()

	reflectutils.MakeDst(&v, typ)

	if v.Kind() != reflect.Struct {
		t.Fatalf("expected struct kind after MakeDst, got %v", v.Kind())
	}
}

func TestMakeDst_NonPointer(t *testing.T) {
	var dst testStruct
	v := reflect.ValueOf(&dst).Elem()
	typ := reflect.TypeFor[testStruct]()

	reflectutils.MakeDst(&v, typ)

	if v.Kind() != reflect.Struct {
		t.Fatalf("expected struct kind unchanged after MakeDst, got %v", v.Kind())
	}
}

func TestIsPrimitive(t *testing.T) {
	tests := []struct {
		name string
		kind reflect.Kind
		want bool
	}{
		{"int", reflect.Int, true},
		{"int8", reflect.Int8, true},
		{"int16", reflect.Int16, true},
		{"int32", reflect.Int32, true},
		{"int64", reflect.Int64, true},
		{"uint", reflect.Uint, true},
		{"uint8", reflect.Uint8, true},
		{"uint16", reflect.Uint16, true},
		{"uint32", reflect.Uint32, true},
		{"uint64", reflect.Uint64, true},
		{"float32", reflect.Float32, true},
		{"float64", reflect.Float64, true},
		{"bool", reflect.Bool, true},
		{"string", reflect.String, true},
		{"struct", reflect.Struct, false},
		{"slice", reflect.Slice, false},
		{"map", reflect.Map, false},
		{"pointer", reflect.Pointer, false},
		{"interface", reflect.Interface, false},
		{"chan", reflect.Chan, false},
		{"func", reflect.Func, false},
		{"array", reflect.Array, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reflectutils.IsPrimitive(tt.kind)
			if got != tt.want {
				t.Errorf("IsPrimitive(%v) = %v, want %v", tt.kind, got, tt.want)
			}
		})
	}
}
