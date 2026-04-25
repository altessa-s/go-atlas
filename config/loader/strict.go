// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package loader

import "errors"

// Strict mode sentinel errors.
// These errors are returned when WithStrict() is enabled and the loader
// encounters conditions that would otherwise be silently ignored.
var (
	// ErrUndefinedEnvVar is returned when a referenced environment variable is not defined.
	ErrUndefinedEnvVar = errors.New("undefined environment variable")

	// ErrUnsupportedFieldType is returned when a field has a type that cannot be set from a string value.
	ErrUnsupportedFieldType = errors.New("unsupported field type")

	// ErrFieldAssignment is returned when a field value assignment fails due to type incompatibility.
	ErrFieldAssignment = errors.New("field assignment failed")

	// ErrFieldNotFound is returned when a referenced field does not exist in the configuration struct.
	ErrFieldNotFound = errors.New("field not found")
)
