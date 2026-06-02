// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package orderby

import "errors"

// Sentinel errors for the orderby package. Wrap callers use [errors.Is] /
// [errors.As] for classification; the message attached at the wrap site
// carries the offending token, position, or limit.
var (
	// ErrParseFailed indicates the input could not be tokenized into an
	// AIP-132 order_by sequence (e.g. a clause produced more than the
	// expected field+direction tokens).
	ErrParseFailed = errors.New("failed to parse order_by expression")

	// ErrEmptyClause indicates a clause between commas was empty or
	// whitespace-only (e.g. "a,,b" or a leading/trailing comma).
	ErrEmptyClause = errors.New("empty order_by clause")

	// ErrInvalidFieldPath indicates a field path that does not match the
	// AIP-132 grammar — a non-ident character, a leading digit, or an empty
	// segment from consecutive dots.
	ErrInvalidFieldPath = errors.New("invalid field path")

	// ErrInvalidDirection indicates a direction token other than asc or desc.
	// Matching is case-insensitive by default; [WithCaseSensitiveDirection]
	// restricts it to the lowercase tokens only.
	ErrInvalidDirection = errors.New("invalid sort direction")

	// ErrExpressionTooLong indicates the raw input exceeds the configured
	// MaxExpressionLength.
	ErrExpressionTooLong = errors.New("order_by expression length exceeds maximum allowed")

	// ErrMaxKeysExceeded indicates the number of sort keys exceeds the
	// configured MaxKeys.
	ErrMaxKeysExceeded = errors.New("number of sort keys exceeds maximum allowed")

	// ErrMaxFieldPathDepthExceeded indicates a single key's dotted field
	// path has more segments than MaxFieldPathDepth.
	ErrMaxFieldPathDepthExceeded = errors.New("field path depth exceeds maximum allowed")

	// ErrMaxFieldNameLengthExceeded indicates a single key's field path
	// is longer than MaxFieldNameLength.
	ErrMaxFieldNameLengthExceeded = errors.New("field name length exceeds maximum allowed")

	// ErrDuplicateKey indicates the same field path appears more than once
	// in an order_by string. AIP-132 does not assign a meaningful semantic
	// to `"slug, slug desc"` — backends either silently let the later
	// entry win (MongoDB, RediSearch) or treat the input as malformed
	// (Meilisearch). Rejecting duplicates at parse time gives the caller
	// one consistent error regardless of the target backend.
	ErrDuplicateKey = errors.New("duplicate sort key")

	// ErrFieldNotAllowed indicates a key references a field absent from the
	// configured allow-list.
	ErrFieldNotAllowed = errors.New("field not allowed")

	// ErrAllowlistRequired indicates a translator was constructed with
	// WithUntrustedInput but no WithAllowedFields allow-list was supplied.
	// Translating untrusted order_by without an allow-list would let the
	// caller sort on any indexed field, so the call refuses to proceed.
	ErrAllowlistRequired = errors.New("allowlist is required for untrusted input")

	// ErrTooManySortKeys is returned by translators that do not support
	// multi-key ordering (notably RediSearch, whose FT.SEARCH ... SORTBY
	// accepts a single field).
	ErrTooManySortKeys = errors.New("backend supports only a single sort key")
)
