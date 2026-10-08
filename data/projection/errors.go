// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package projection

import "errors"

// Sentinel errors for the projection package. Callers classify with
// [errors.Is]; the message attached at the wrap site carries the offending
// path or limit.
var (
	// ErrEmptyClause indicates a path between commas was empty or
	// whitespace-only (e.g. "a,,b" or a leading/trailing comma).
	ErrEmptyClause = errors.New("empty fields clause")

	// ErrInvalidFieldPath indicates a path that does not match the grammar:
	// a non-identifier character, a leading digit, or an empty segment from
	// consecutive dots. Translators also return it for a storage name the
	// backend cannot address (a SQL column must be `col` or `table.col`).
	ErrInvalidFieldPath = errors.New("invalid field path")

	// ErrUnsupportedPath indicates AIP-161 syntax that is valid in a field
	// mask but deliberately unsupported by this package: wildcard segments
	// ("items.*.name", "a,*"), backtick-quoted map keys and numeric
	// segments ("items.0").
	ErrUnsupportedPath = errors.New("unsupported field path syntax")

	// ErrExpressionTooLong indicates the raw input exceeds the configured
	// MaxExpressionLength.
	ErrExpressionTooLong = errors.New("fields expression length exceeds maximum allowed")

	// ErrMaxPathsExceeded indicates the number of paths exceeds the
	// configured MaxPaths. The limit applies before deduplication.
	ErrMaxPathsExceeded = errors.New("number of field paths exceeds maximum allowed")

	// ErrMaxFieldPathDepthExceeded indicates a path has more dot-separated
	// segments than MaxFieldPathDepth.
	ErrMaxFieldPathDepthExceeded = errors.New("field path depth exceeds maximum allowed")

	// ErrMaxFieldNameLengthExceeded indicates a path is longer than
	// MaxFieldNameLength.
	ErrMaxFieldNameLengthExceeded = errors.New("field name length exceeds maximum allowed")

	// ErrFieldNotAllowed indicates a requested path is absent from the
	// allow-list, or overlaps a denied field — equal to it, an ancestor of
	// it, or a descendant of it.
	ErrFieldNotAllowed = errors.New("field not allowed")

	// ErrAllowlistRequired indicates a translator was constructed with
	// WithUntrustedInput but without a non-empty WithAllowedFields list.
	ErrAllowlistRequired = errors.New("allowlist is required for untrusted input")

	// ErrConflictingPolicy indicates translator options that contradict each
	// other: a required or default field that is denied, or a default field
	// outside the allow-list.
	ErrConflictingPolicy = errors.New("conflicting projection policy")

	// ErrDefaultFieldsRequired indicates the projection for an empty request
	// ("all authorized fields") cannot be derived from the policy and must be
	// configured with WithDefaultFields — for example an allow-list whose
	// roots overlap a denied field, or a SQL backend with denied fields.
	ErrDefaultFieldsRequired = errors.New("default fields are required")
)
