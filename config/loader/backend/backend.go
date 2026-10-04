// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package backend

import (
	"io"
	"reflect"
)

// Decoder is the interface that should be implemented by a backend to decode the data from the reader.
// Implementations should parse the configuration format into the provided struct.
type Decoder interface {
	Decode(reader io.Reader, in any) error
}

// Backend is the interface that should be implemented by a configuration backend.
// It provides format-specific decoding and file extension information.
type Backend interface {
	Decoder

	// FileExtensions should return the file extensions that the backend supports.
	FileExtensions() []string

	// StructTagName should return the struct tag name for the backend.
	// This is used to decode the struct tags.
	StructTagName() string
}

// Preprocessor is the interface that should be implemented by a backend that supports
// pre-processing of the configuration content before decoding.
type Preprocessor interface {
	// Preprocess performs pre-processing on the provided content.
	// currentDir is the directory of the file being processed.
	// rootDir is the root directory for security checks.
	Preprocess(content, currentDir, rootDir string) (string, error)
}

// KeyDecoder is implemented by a backend that can report which values a
// document sets. The loader uses it to tell an explicit false, 0 or "" from an
// omitted key, so default tags apply only to the latter.
type KeyDecoder interface {
	// DecodeKeys parses the document and returns its root value; an empty
	// document is a null value.
	DecodeKeys(reader io.Reader) (KeyNode, error)
}

// KeyNode is a value of a parsed document. Its children are bound to
// destination types on demand, exactly the way the backend's Decode binds them:
// struct fields by the backend's key, embedding and case rules; map keys
// decoded into the map's key type; sequence elements by the position they
// take in the decoded slice.
type KeyNode interface {
	// IsNull reports whether the value is an explicit null.
	IsNull() bool

	// Fields binds the value to struct type t and returns the values of the
	// fields it sets, keyed by field index in t. An anonymous or inline field
	// whose fields the backend flattens into t maps to a node whose Fields
	// returns them. It returns false when the value does not bind to t
	// field by field, e.g. when t decodes itself through an unmarshaler.
	Fields(t reflect.Type) (map[int]KeyNode, bool)

	// Entries binds the value to map type t and returns its entries keyed by
	// the decoded key (a value of t's key type), or false when it does not
	// bind to t.
	Entries(t reflect.Type) (map[any]KeyNode, bool)

	// Elems binds the value to slice or array type t and returns its elements
	// in decoded order, or false when it does not bind to t.
	Elems(t reflect.Type) ([]KeyNode, bool)
}
