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

	// ErrFunctionNotAllowed indicates that a CEL function (built-in or
	// custom) is not in the allowed list configured via
	// [WithAllowedFunctions]. Operators (==, !=, <, &&, ||, !, in) and
	// the has() macro are part of the baseline grammar and are not
	// subject to this whitelist.
	ErrFunctionNotAllowed = errors.New("function not allowed")

	// ErrAllowlistRequired indicates a translator/evaluator was constructed
	// with [WithUntrustedInput] but no [WithAllowedFields] allowlist was
	// supplied. Translating untrusted CEL without an allowlist would let the
	// caller filter on any field — including ones the application never
	// intended to expose — so the call refuses to proceed.
	ErrAllowlistRequired = errors.New("allowlist is required for untrusted input")

	// ErrMaxDepthExceeded indicates the expression nesting exceeds the configured limit.
	ErrMaxDepthExceeded = errors.New("expression depth exceeds maximum allowed")

	// ErrInvalidExpression indicates a structurally invalid expression.
	ErrInvalidExpression = errors.New("invalid expression structure")

	// ErrUnsupportedType indicates an unsupported literal or value type.
	ErrUnsupportedType = errors.New("unsupported type")

	// ErrFieldTypeMismatch indicates a literal in the filter is not assignable
	// to the kind declared for the field via [WithFieldTypes].
	ErrFieldTypeMismatch = errors.New("filter value type does not match field type")

	// ErrEnumValueNotAllowed indicates an integer literal in the filter is not in
	// the value set declared for the field via [WithEnumValues].
	ErrEnumValueNotAllowed = errors.New("filter value is not an allowed enum value")

	// ErrEmptyExpression indicates an empty or whitespace-only expression was provided.
	ErrEmptyExpression = errors.New("expression cannot be empty")

	// ErrInvalidRegex indicates a regex pattern that is invalid or too complex.
	ErrInvalidRegex = errors.New("invalid or unsafe regex pattern")

	// ErrMaxOperationsExceeded indicates the expression requires more operations
	// than the configured limit allows.
	ErrMaxOperationsExceeded = errors.New("operation count exceeds maximum allowed")

	// ErrExpressionTooLong indicates the expression exceeds the configured maximum length.
	ErrExpressionTooLong = errors.New("expression length exceeds maximum allowed")
)
