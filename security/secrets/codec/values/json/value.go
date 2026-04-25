// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package json

import (
	"encoding/json"

	"github.com/altessa-s/go-atlas/security/secrets/codec"
)

// Ensure ValueDecoder implements codec.ValueDecoder interface
var _ codec.ValueDecoder[any] = (*ValueDecoder[any])(nil)

// ValueDecoder implements codec.ValueDecoder using JSON encoding/decoding.
// It provides a simple JSON-based serialization for values while maintaining
// compatibility with the base64 package naming convention.
type ValueDecoder[T any] struct{}

// NewValueDecoder creates a new ValueDecoder instance for the specified type.
// This function provides a convenient way to create ValueDecoder instances
// with proper type constraints.
//
// Returns a new ValueDecoder instance configured for type T.
//
// Example:
//
//	// Create decoder for string values
//	decoder := NewValueDecoder[string]()
//
//	// Create decoder for complex structures
//	decoder := NewValueDecoder[map[string]any()
func NewValueDecoder[T any]() *ValueDecoder[T] {
	return &ValueDecoder[T]{}
}

// Decode transforms binary data into a value of type T using JSON unmarshaling.
// The key parameter is currently unused but provided for interface
// compatibility and potential future extensions.
//
// Returns the decoded value of type T and an error if JSON unmarshaling fails.
//
// Example:
//
//	decoder := NewValueDecoder[string]()
//	value, err := decoder.Decode([]byte(`"hello world"`))
//	if err != nil {
//		// Handle error
//	}
//	// value is now "hello world"
func (v *ValueDecoder[T]) Decode(data []byte) (T, error) {
	var value T
	err := json.Unmarshal(data, &value)
	if err != nil {
		var zero T
		return zero, err
	}
	return value, nil
}

// Encode transforms a value of type T into binary data using JSON marshaling.
// The key parameter is currently unused but provided for interface
// compatibility and potential future extensions.
//
// Returns the encoded binary JSON data and an error if JSON marshaling fails.
//
// Example:
//
//	decoder := NewValueDecoder[string]()
//	data, err := decoder.Encode("hello world")
//	if err != nil {
//		// Handle error
//	}
//	// data is now []byte(`"hello world"`)
func (v *ValueDecoder[T]) Encode(value T) ([]byte, error) {
	return json.Marshal(value)
}
