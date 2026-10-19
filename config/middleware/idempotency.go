// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package middlewareconfig

import (
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// IdempotencyKeyFormat selects the validator idempotency keys must pass,
// shared by the gRPC interceptor and the HTTP middleware.
type IdempotencyKeyFormat string

const (
	// IdempotencyKeyFormatUUIDv4 accepts only a lowercase UUID v4 (the
	// default).
	IdempotencyKeyFormatUUIDv4 IdempotencyKeyFormat = "uuidv4"
	// IdempotencyKeyFormatOpaque accepts any key of visible ASCII
	// characters up to the maximum key length (client request IDs).
	IdempotencyKeyFormatOpaque IdempotencyKeyFormat = "opaque"
	// IdempotencyKeyFormatPattern accepts keys a regular expression matches
	// in full, up to the maximum key length.
	IdempotencyKeyFormatPattern IdempotencyKeyFormat = "pattern"
)

// AllIdempotencyKeyFormats returns all valid IdempotencyKeyFormat values.
func AllIdempotencyKeyFormats() []IdempotencyKeyFormat {
	return []IdempotencyKeyFormat{IdempotencyKeyFormatUUIDv4, IdempotencyKeyFormatOpaque, IdempotencyKeyFormatPattern}
}

// IdempotencyKeyFormatRule returns the validation rule accepting exactly the
// IdempotencyKeyFormat values.
func IdempotencyKeyFormatRule() ozzo_rules.OneOfRule[IdempotencyKeyFormat] {
	return ozzo_rules.OneOf(AllIdempotencyKeyFormats()...)
}

// IdempotencyKeyRules returns the field rules the gRPC and HTTP idempotency
// configs share: format a known [IdempotencyKeyFormat] when set (an empty
// format, as a config built in code leaves it, is uuidv4), pattern required
// for the pattern format and a valid regular expression, maxLen not
// negative, every mandatory entry non-empty and every mandatoryPatterns
// entry a non-empty valid regular expression. The pointers must address
// fields of the struct being validated, so errors keep their field paths.
func IdempotencyKeyRules(
	format *IdempotencyKeyFormat, pattern *string, maxLen *int, mandatory, mandatoryPatterns *[]string,
) []*validation.FieldRules {
	return []*validation.FieldRules{
		validation.Field(format, validation.When(*format != "", IdempotencyKeyFormatRule())),
		validation.Field(pattern,
			validation.When(*format == IdempotencyKeyFormatPattern, validation.Required),
			ozzo_rules.Regex()),
		validation.Field(maxLen, validation.Min(0)),
		validation.Field(mandatory, validation.Each(validation.Required)),
		validation.Field(mandatoryPatterns, validation.Each(validation.Required, ozzo_rules.Regex())),
	}
}
