// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package json_test

import (
	"reflect"
	"testing"

	"github.com/altessa-s/go-atlas/security/secrets/codec/values/json"
)

func TestValueDecoder_String_EncodeDecode(t *testing.T) {
	decoder := json.NewValueDecoder[string]()
	original := "hello"

	encoded, err := decoder.Encode(original)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	decoded, err := decoder.Decode(encoded)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if decoded != original {
		t.Errorf("Roundtrip failed: got %q, want %q", decoded, original)
	}
}

func TestValueDecoder_Struct_EncodeDecode(t *testing.T) {
	type TestStruct struct {
		Name string
		Age  int
	}

	decoder := json.NewValueDecoder[TestStruct]()
	original := TestStruct{Name: "Antonio", Age: 30}

	encoded, err := decoder.Encode(original)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	decoded, err := decoder.Decode(encoded)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if !reflect.DeepEqual(decoded, original) {
		t.Errorf("Roundtrip failed: got %+v, want %+v", decoded, original)
	}
}

func TestValueDecoder_Decode_InvalidJSON(t *testing.T) {
	decoder := json.NewValueDecoder[map[string]any]()
	_, err := decoder.Decode([]byte("{invalid json}"))
	if err == nil {
		t.Error("Decode() expected error for invalid JSON, got nil")
	}
}

func TestValueDecoder_Map(t *testing.T) {
	decoder := json.NewValueDecoder[map[string]any]()
	original := map[string]any{
		"name":   "Antonio",
		"age":    float64(30),
		"active": true,
	}

	encoded, err := decoder.Encode(original)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	decoded, err := decoder.Decode(encoded)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if !reflect.DeepEqual(decoded, original) {
		t.Errorf("Roundtrip failed: got %+v, want %+v", decoded, original)
	}
}
