// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mapslice_test

import (
	"reflect"
	"testing"

	"github.com/altessa-s/go-atlas/domain/converter/codec/mapslice"
)

func TestValues(t *testing.T) {
	src := reflect.ValueOf(map[string]int{"a": 1, "b": 2})
	dst := reflect.New(reflect.SliceOf(reflect.TypeFor[int]())).Elem()

	mapslice.Values("test", src, dst, func(_ string, _, _ reflect.Value) {})

	if dst.Len() != 2 {
		t.Fatalf("expected slice len 2, got %d", dst.Len())
	}
}

func TestValues_PassThrough(t *testing.T) {
	src := reflect.ValueOf("hello")
	var dst string
	dstVal := reflect.ValueOf(&dst).Elem()

	nextCalled := false
	mapslice.Values("test", src, dstVal, func(_ string, _, _ reflect.Value) {
		nextCalled = true
	})

	if !nextCalled {
		t.Fatal("next should be called for non-map types")
	}
}

func TestValues_NotNil(t *testing.T) {
	if mapslice.Values == nil {
		t.Fatal("Values is nil")
	}
}

func TestKeys(t *testing.T) {
	src := reflect.ValueOf(map[string]int{"a": 1, "b": 2})
	dst := reflect.New(reflect.SliceOf(reflect.TypeFor[string]())).Elem()

	mapslice.Keys("test", src, dst, func(_ string, _, _ reflect.Value) {})

	if dst.Len() != 2 {
		t.Fatalf("expected slice len 2, got %d", dst.Len())
	}
}

func TestKeys_PassThrough(t *testing.T) {
	src := reflect.ValueOf("hello")
	var dst string
	dstVal := reflect.ValueOf(&dst).Elem()

	nextCalled := false
	mapslice.Keys("test", src, dstVal, func(_ string, _, _ reflect.Value) {
		nextCalled = true
	})

	if !nextCalled {
		t.Fatal("next should be called for non-map types")
	}
}

func TestKeys_NotNil(t *testing.T) {
	if mapslice.Keys == nil {
		t.Fatal("Keys is nil")
	}
}
