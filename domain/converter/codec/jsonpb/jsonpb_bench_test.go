// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package jsonpb

import (
	"encoding/json"
	"reflect"
	"testing"

	"google.golang.org/protobuf/types/known/structpb"
)

func BenchmarkNew_RawMessageToStruct(b *testing.B) {
	raw := json.RawMessage(`{"key":"value","num":42}`)
	codec := New()
	src := reflect.ValueOf(&raw)
	handler := func(_ string, _, _ reflect.Value) {}

	for b.Loop() {
		var dst *structpb.Struct
		dstVal := reflect.ValueOf(&dst).Elem()
		codec("Metadata", src, dstVal, handler)
	}
}

func BenchmarkNew_StructToRawMessage(b *testing.B) {
	pb, _ := structpb.NewStruct(map[string]any{"key": "value", "num": float64(42)})
	codec := New()
	src := reflect.ValueOf(pb)
	handler := func(_ string, _, _ reflect.Value) {}

	for b.Loop() {
		var dst *json.RawMessage
		dstVal := reflect.ValueOf(&dst).Elem()
		codec("Metadata", src, dstVal, handler)
	}
}

func BenchmarkNew_PassThrough(b *testing.B) {
	codec := New()
	src := reflect.ValueOf("hello")
	handler := func(_ string, _, _ reflect.Value) {}

	for b.Loop() {
		var dst string
		dstVal := reflect.ValueOf(&dst).Elem()
		codec("Name", src, dstVal, handler)
	}
}
