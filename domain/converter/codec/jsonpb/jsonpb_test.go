// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package jsonpb

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"google.golang.org/protobuf/types/known/structpb"
)

func TestNew_RawMessageToStruct(t *testing.T) {
	raw := json.RawMessage(`{"key":"value","num":42}`)
	codec := New()

	src := reflect.ValueOf(&raw)
	var dst *structpb.Struct
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("Metadata", src, dstVal, func(_ string, _, _ reflect.Value) {
		t.Fatal("next should not be called")
	})

	require.NotNil(t, dst)
	assert.Equal(t, "value", dst.Fields["key"].GetStringValue())
	assert.Equal(t, float64(42), dst.Fields["num"].GetNumberValue())
}

func TestNew_StructToRawMessage(t *testing.T) {
	pb, err := structpb.NewStruct(map[string]any{"key": "value", "num": float64(42)})
	require.NoError(t, err)

	codec := New()

	src := reflect.ValueOf(pb)
	var dst *json.RawMessage
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("Metadata", src, dstVal, func(_ string, _, _ reflect.Value) {
		t.Fatal("next should not be called")
	})

	require.NotNil(t, dst)

	var m map[string]any
	require.NoError(t, json.Unmarshal(*dst, &m))
	assert.Equal(t, "value", m["key"])
	assert.Equal(t, float64(42), m["num"])
}

func TestNew_RoundTrip(t *testing.T) {
	original := json.RawMessage(`{"a":"b","c":123}`)
	codec := New()

	// json.RawMessage -> structpb.Struct
	src1 := reflect.ValueOf(&original)
	var intermediate *structpb.Struct
	dstVal1 := reflect.ValueOf(&intermediate).Elem()

	codec("Data", src1, dstVal1, func(_ string, _, _ reflect.Value) {
		t.Fatal("next should not be called")
	})

	require.NotNil(t, intermediate)

	// structpb.Struct -> json.RawMessage
	src2 := reflect.ValueOf(intermediate)
	var result *json.RawMessage
	dstVal2 := reflect.ValueOf(&result).Elem()

	codec("Data", src2, dstVal2, func(_ string, _, _ reflect.Value) {
		t.Fatal("next should not be called")
	})

	require.NotNil(t, result)

	var originalMap, resultMap map[string]any
	require.NoError(t, json.Unmarshal(original, &originalMap))
	require.NoError(t, json.Unmarshal(*result, &resultMap))
	assert.Equal(t, originalMap, resultMap)
}

func TestNew_NilRawMessage(t *testing.T) {
	codec := New()

	var raw *json.RawMessage
	src := reflect.ValueOf(raw)
	var dst *structpb.Struct
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("Metadata", src, dstVal, func(_ string, _, _ reflect.Value) {
		t.Fatal("next should not be called")
	})

	assert.Nil(t, dst)
}

func TestNew_NilStruct(t *testing.T) {
	codec := New()

	var pb *structpb.Struct
	src := reflect.ValueOf(pb)
	var dst *json.RawMessage
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("Metadata", src, dstVal, func(_ string, _, _ reflect.Value) {
		t.Fatal("next should not be called")
	})

	assert.Nil(t, dst)
}

func TestNew_EmptyRawMessage(t *testing.T) {
	codec := New()

	raw := json.RawMessage{}
	src := reflect.ValueOf(&raw)
	var dst *structpb.Struct
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("Metadata", src, dstVal, func(_ string, _, _ reflect.Value) {
		t.Fatal("next should not be called")
	})

	assert.Nil(t, dst, "empty RawMessage should not produce a Struct")
}

func TestNew_InvalidJSON(t *testing.T) {
	codec := New()

	raw := json.RawMessage(`{invalid`)
	src := reflect.ValueOf(&raw)
	var dst *structpb.Struct
	dstVal := reflect.ValueOf(&dst).Elem()

	// Invalid JSON cannot be converted - codec returns false, next is called
	nextCalled := false
	codec("Metadata", src, dstVal, func(_ string, _, _ reflect.Value) {
		nextCalled = true
	})

	assert.True(t, nextCalled, "next should be called for invalid JSON")
}

func TestNew_PassThrough(t *testing.T) {
	codec := New()

	src := reflect.ValueOf("hello")
	var dst string
	dstVal := reflect.ValueOf(&dst).Elem()

	nextCalled := false
	codec("Name", src, dstVal, func(_ string, _, _ reflect.Value) {
		nextCalled = true
	})

	assert.True(t, nextCalled, "next should be called for non-matching types")
}

func TestNew_NonPointer_RawMessageToStruct(t *testing.T) {
	raw := json.RawMessage(`{"key":"value"}`)
	codec := New()

	src := reflect.ValueOf(raw)
	var dst structpb.Struct
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("Metadata", src, dstVal, func(_ string, _, _ reflect.Value) {
		t.Fatal("next should not be called")
	})

	assert.Equal(t, "value", dst.Fields["key"].GetStringValue())
}

func TestNew_NonPointer_StructToRawMessage(t *testing.T) {
	pb, err := structpb.NewStruct(map[string]any{"key": "value"})
	require.NoError(t, err)

	codec := New()

	// Use reflect.New so the structpb.Struct is addressable (like real converter usage)
	// without triggering copylocks.
	val := reflect.New(structpbType)
	val.Elem().Set(reflect.ValueOf(pb).Elem())
	src := val.Elem()
	var dst json.RawMessage
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("Metadata", src, dstVal, func(_ string, _, _ reflect.Value) {
		t.Fatal("next should not be called")
	})

	var m map[string]any
	require.NoError(t, json.Unmarshal(dst, &m))
	assert.Equal(t, "value", m["key"])
}

func TestNew_IgnoreNil_RawMessage(t *testing.T) {
	codec := New(WithIgnoreNil())

	var raw *json.RawMessage
	src := reflect.ValueOf(raw)
	var dst *structpb.Struct
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("Metadata", src, dstVal, func(_ string, _, _ reflect.Value) {
		t.Fatal("next should not be called")
	})

	assert.Nil(t, dst)
}

func TestNew_IgnoreNil_Struct(t *testing.T) {
	codec := New(WithIgnoreNil())

	var pb *structpb.Struct
	src := reflect.ValueOf(pb)
	var dst *json.RawMessage
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("Metadata", src, dstVal, func(_ string, _, _ reflect.Value) {
		t.Fatal("next should not be called")
	})

	assert.Nil(t, dst)
}

func TestNew_NestedObject(t *testing.T) {
	raw := json.RawMessage(`{"outer":{"inner":"deep"},"list":[1,2,3]}`)
	codec := New()

	src := reflect.ValueOf(&raw)
	var dst *structpb.Struct
	dstVal := reflect.ValueOf(&dst).Elem()

	codec("Metadata", src, dstVal, func(_ string, _, _ reflect.Value) {
		t.Fatal("next should not be called")
	})

	require.NotNil(t, dst)

	inner := dst.Fields["outer"].GetStructValue()
	require.NotNil(t, inner)
	assert.Equal(t, "deep", inner.Fields["inner"].GetStringValue())

	list := dst.Fields["list"].GetListValue()
	require.NotNil(t, list)
	assert.Len(t, list.Values, 3)
}
