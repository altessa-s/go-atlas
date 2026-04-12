// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package json_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/security/secrets/codec/values/json"
)

func TestValueDecoder_String_EncodeDecode(t *testing.T) {
	decoder := json.NewValueDecoder[string]()
	original := "hello"

	encoded, err := decoder.Encode(original)
	require.NoError(t, err)

	decoded, err := decoder.Decode(encoded)
	require.NoError(t, err)
	require.Equal(t, original, decoded)
}

func TestValueDecoder_Struct_EncodeDecode(t *testing.T) {
	type TestStruct struct {
		Name string
		Age  int
	}

	decoder := json.NewValueDecoder[TestStruct]()
	original := TestStruct{Name: "Antonio", Age: 30}

	encoded, err := decoder.Encode(original)
	require.NoError(t, err)

	decoded, err := decoder.Decode(encoded)
	require.NoError(t, err)
	require.Equal(t, original, decoded)
}

func TestValueDecoder_Decode_InvalidJSON(t *testing.T) {
	decoder := json.NewValueDecoder[map[string]any]()
	_, err := decoder.Decode([]byte("{invalid json}"))
	require.Error(t, err)
}

func TestValueDecoder_Map(t *testing.T) {
	decoder := json.NewValueDecoder[map[string]any]()
	original := map[string]any{
		"name":   "Antonio",
		"age":    float64(30),
		"active": true,
	}

	encoded, err := decoder.Encode(original)
	require.NoError(t, err)

	decoded, err := decoder.Decode(encoded)
	require.NoError(t, err)
	require.Equal(t, original, decoded)
}
