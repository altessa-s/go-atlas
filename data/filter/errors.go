// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter

import "errors"

// Sentinel errors for the filter package.
var (
	// ErrParseFailed indicates that CEL expression parsing failed.
	ErrParseFailed = errors.New("failed to parse CEL expression")

	// ErrUnsupportedOperation indicates an operation that cannot be translated.
	ErrUnsupportedOperation = errors.New("unsupported operation")

	// ErrFieldNotAllowed indicates that a field is not in the allowed list.
	ErrFieldNotAllowed = errors.New("field not allowed")

	// ErrMaxDepthExceeded indicates the expression nesting exceeds the configured limit.
	ErrMaxDepthExceeded = errors.New("expression depth exceeds maximum allowed")

	// ErrInvalidExpression indicates a structurally invalid expression.
	ErrInvalidExpression = errors.New("invalid expression structure")

	// ErrUnsupportedType indicates an unsupported literal or value type.
	ErrUnsupportedType = errors.New("unsupported type")

	// ErrEmptyExpression indicates an empty or whitespace-only expression was provided.
	ErrEmptyExpression = errors.New("expression cannot be empty")

	// ErrInvalidRegex indicates a regex pattern that is invalid or too complex.
	ErrInvalidRegex = errors.New("invalid or unsafe regex pattern")
)
