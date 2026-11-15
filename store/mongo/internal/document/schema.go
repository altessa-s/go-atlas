// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package document

// Field processing constants
const (
	// NestedEncryptionKey is used in encryption tag values to indicate nested field encryption
	NestedEncryptionKey = "nested"
)

// NestedStructType represents the type of nested structure in a field
type NestedStructType int

// Constants for nested structure types
const (
	// NoNesting indicates the field does not contain a nested struct.
	NoNesting NestedStructType = iota
	// PointerStruct indicates the field is a pointer to a struct.
	PointerStruct
	// SliceStruct indicates the field is a slice of structs.
	SliceStruct
	// MapStruct indicates the field is a map with struct values.
	MapStruct
)

// EncryptionField defines how a specific struct field should be encrypted.
type EncryptionField struct {
	// FieldName is the name of the struct field to encrypt (case-sensitive)
	FieldName string
	// Algorithm is the encryption algorithm: "deterministic" or "random"
	Algorithm string
	// KeyAltName is the alternative name of the data key to use for encryption
	KeyAltName string
}

// EncryptionModel defines encryption configuration for a specific struct type.
type EncryptionModel struct {
	Type any
	// Fields defines which fields should be encrypted and how
	Fields []EncryptionField
}
