// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package reasoncode

import (
	"strings"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

// Canonical reason codes for the standard protovalidate rules.
const (
	// Unknown is returned for a rule that has no mapping.
	Unknown = "UNKNOWN"

	// InvalidMinLengthOrValue is a value or length below its minimum bound.
	InvalidMinLengthOrValue = "INVALID_MIN_LENGTH_OR_VALUE"
	// InvalidMaxLengthOrValue is a value or length above (or not equal to) its bound.
	InvalidMaxLengthOrValue = "INVALID_MAX_LENGTH_OR_VALUE"

	// InvalidFormatEmail is an invalid email format.
	InvalidFormatEmail = "INVALID_FORMAT_EMAIL"
	// InvalidFormatUUID is an invalid UUID format.
	InvalidFormatUUID = "INVALID_FORMAT_UUID"
	// InvalidFormatRegex is a value not matching a required pattern.
	InvalidFormatRegex = "INVALID_FORMAT_REGEX"
	// InvalidFormatURL is an invalid URL or URI format.
	InvalidFormatURL = "INVALID_FORMAT_URL"

	// InvalidEnumValue is a value outside the set allowed by an enum rule.
	InvalidEnumValue = "INVALID_ENUM_VALUE"
)

const (
	// ruleIDRequired is the rule ID protovalidate reports for a missing field.
	ruleIDRequired = "required"
	// requiredSuffix is appended to the field name for a required violation.
	requiredSuffix = "_REQUIRED"
	// requiredFallback is used when the field name cannot be determined.
	requiredFallback = "REQUIRED"
)

// standardFullID maps type-qualified standard rule IDs whose trailing segment is
// ambiguous and so cannot be matched by suffix alone (e.g. "const" and "in" are
// shared by enum and scalar rules).
var standardFullID = map[string]string{
	"enum.defined_only": InvalidEnumValue,
	"enum.in":           InvalidEnumValue,
	"enum.const":        InvalidEnumValue,
	"string.email":      InvalidFormatEmail,
	"string.uuid":       InvalidFormatUUID,
	"string.pattern":    InvalidFormatRegex,
	"bytes.pattern":     InvalidFormatRegex,
	"string.uri":        InvalidFormatURL,
	"string.uri_ref":    InvalidFormatURL,
}

// standardSuffix maps the numeric and size families by the segment after the
// final ".", collapsing the per-type rules (int64.gte, uint32.gte, ...) onto a
// single code.
var standardSuffix = map[string]string{
	"gte":       InvalidMinLengthOrValue,
	"gt":        InvalidMinLengthOrValue,
	"min_len":   InvalidMinLengthOrValue,
	"min_items": InvalidMinLengthOrValue,
	"min_pairs": InvalidMinLengthOrValue,
	"min_bytes": InvalidMinLengthOrValue,

	"lte":       InvalidMaxLengthOrValue,
	"lt":        InvalidMaxLengthOrValue,
	"max_len":   InvalidMaxLengthOrValue,
	"max_items": InvalidMaxLengthOrValue,
	"max_pairs": InvalidMaxLengthOrValue,
	"max_bytes": InvalidMaxLengthOrValue,
	"len":       InvalidMaxLengthOrValue,
	"len_bytes": InvalidMaxLengthOrValue,
}

// rangeRuleSuffixes are the combined numeric range rules protovalidate emits when
// a field declares both a lower and an upper bound. A single violation covers
// both bounds, so its code cannot be told from the rule ID alone.
var rangeRuleSuffixes = map[string]struct{}{
	"gte_lte": {},
	"gt_lt":   {},
	"gte_lt":  {},
	"gt_lte":  {},
}

// Violation is the information about a single failed validation rule from which a
// reason code is derived. The caller (e.g. the protovalidate integration)
// implements it to adapt its concrete violation type, keeping this package free
// of any validation-library dependency.
type Violation interface {
	// RuleID is the protovalidate rule ID, e.g. "required" or "int64.gte_lte".
	RuleID() string
	// FieldName is the last segment of the violated field path (e.g. "userName"),
	// used to build "{FIELD}_REQUIRED".
	FieldName() string
	// NumericValue is the offending numeric field value; ok is false when the
	// field is not numeric or cannot be read.
	NumericValue() (value float64, ok bool)
	// LowerBound is the rule's lower numeric bound and whether it is inclusive
	// (gte) or exclusive (gt); ok is false when the rule declares none. It is used
	// to tell min from max for a combined range rule.
	LowerBound() (bound float64, inclusive bool, ok bool)
}

// Resolver maps a protovalidate rule ID to a canonical reason code, consulting a
// caller-supplied catalog before the built-in standard rules.
type Resolver struct {
	catalog         map[string]string
	catalogPrefixes []string
}

// NewResolver returns a Resolver that consults catalog (rule ID to canonical
// reason code) before the built-in standard rules. catalog may be nil.
//
// catalogPrefixes name rule-ID namespaces the caller owns. A rule whose ID has
// one of these prefixes but is absent from catalog resolves to [Unknown] instead
// of falling through to the standard rules, so the caller's own rules cannot be
// mapped by accident.
func NewResolver(catalog map[string]string, catalogPrefixes ...string) *Resolver {
	return &Resolver{catalog: catalog, catalogPrefixes: catalogPrefixes}
}

// ResolveViolation returns the canonical reason code for a validation violation.
// It handles the rules whose code depends on more than the rule ID — "required"
// (which needs the field name) and combined numeric range rules (which need the
// field value to tell min from max) — and falls back to [Resolver.Resolve] for
// every other rule.
func (r *Resolver) ResolveViolation(v Violation) string {
	ruleID := v.RuleID()
	if ruleID == "" {
		return ""
	}
	if strings.EqualFold(ruleID, ruleIDRequired) {
		return requiredCode(v.FieldName())
	}
	if isRangeRule(ruleID) {
		if value, ok := v.NumericValue(); ok {
			if lower, inclusive, ok := v.LowerBound(); ok {
				return rangeCode(value, lower, inclusive)
			}
		}
	}
	return r.Resolve(ruleID)
}

// Resolve maps a protovalidate rule ID to a canonical reason code using the
// catalog and the standard rules. It handles only rules that depend on the rule
// ID alone; use [Resolver.ResolveViolation] for "required" and numeric range
// rules. The empty rule ID resolves to the empty string.
func (r *Resolver) Resolve(ruleID string) string {
	if ruleID == "" {
		return ""
	}

	if r != nil {
		if code, ok := r.catalog[ruleID]; ok {
			return code
		}
		for _, prefix := range r.catalogPrefixes {
			if strings.HasPrefix(ruleID, prefix) {
				return Unknown
			}
		}
	}

	if code, ok := standardFullID[ruleID]; ok {
		return code
	}
	if code, ok := standardSuffix[ruleSuffix(ruleID)]; ok {
		return code
	}
	return Unknown
}

// requiredCode formats a "required" violation as "{FIELD_NAME}_REQUIRED", falling
// back to "REQUIRED" when the field name is unknown.
func requiredCode(fieldName string) string {
	if fieldName == "" {
		return requiredFallback
	}
	return corestrings.ToScreamingSnakeCase(fieldName) + requiredSuffix
}

// isRangeRule reports whether ruleID is a combined numeric range rule.
func isRangeRule(ruleID string) bool {
	_, ok := rangeRuleSuffixes[ruleSuffix(ruleID)]
	return ok
}

// rangeCode picks the min or max code for a combined range violation from the
// offending value and the lower bound. For an inclusive lower bound value < lower
// is the minimum side; for an exclusive lower bound value <= lower is. Anything
// else is the maximum side.
func rangeCode(value, lower float64, inclusive bool) string {
	if (inclusive && value < lower) || (!inclusive && value <= lower) {
		return InvalidMinLengthOrValue
	}
	return InvalidMaxLengthOrValue
}

// ruleSuffix returns the segment of a rule ID after the last ".".
func ruleSuffix(ruleID string) string {
	if i := strings.LastIndexByte(ruleID, '.'); i >= 0 {
		return ruleID[i+1:]
	}
	return ruleID
}
