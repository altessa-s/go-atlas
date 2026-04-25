// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package backend

import (
	"io"
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
