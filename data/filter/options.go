// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter

import (
	"time"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// DefaultMaxDepth is the default maximum expression nesting depth.
const DefaultMaxDepth = 20

// DefaultMaxRegexLength is the maximum allowed length for user-provided regex patterns.
// Prevents excessive CPU and memory usage from complex regex evaluation.
const DefaultMaxRegexLength = 1024

// DefaultMaxExpressionLength is the maximum allowed length (in bytes) for a CEL expression.
// Prevents excessive memory usage during parsing and LRU cache pollution.
const DefaultMaxExpressionLength = 4096

// DefaultMaxOperations is the maximum number of AST node visits during evaluation.
// Prevents DoS via wide expressions (e.g., 100 OR-ed conditions at depth 1).
const DefaultMaxOperations = 1000

// FieldKind is the declared kind of a queryable field, used by translators
// and the in-memory evaluator to reject filter literals whose Go type does
// not align with the field's schema. See [WithFieldTypes].
type FieldKind uint8

const (
	// FieldKindUnspecified disables type-checking for a field. Equivalent to
	// omitting the field from the [WithFieldTypes] map.
	FieldKindUnspecified FieldKind = iota
	// FieldKindInt accepts CEL integer literals (int64, uint64).
	FieldKindInt
	// FieldKindFloat accepts CEL numeric literals (float64, int64, uint64).
	FieldKindFloat
	// FieldKindString accepts CEL string literals.
	FieldKindString
	// FieldKindBool accepts CEL bool literals.
	FieldKindBool
	// FieldKindBytes accepts CEL bytes literals.
	FieldKindBytes
	// FieldKindTimestamp accepts time.Time values produced by timestamp().
	FieldKindTimestamp
)

// String returns the kind's name for use in error messages.
func (k FieldKind) String() string {
	switch k {
	case FieldKindUnspecified:
		return "unspecified"
	case FieldKindInt:
		return "int"
	case FieldKindFloat:
		return "float"
	case FieldKindString:
		return "string"
	case FieldKindBool:
		return "bool"
	case FieldKindBytes:
		return "bytes"
	case FieldKindTimestamp:
		return "timestamp"
	default:
		return "FieldKind(?)"
	}
}

// TranslatorConfig holds configuration for translators.
type TranslatorConfig struct {
	allowedFields  map[string]struct{}
	fieldMapping   map[string]string
	fieldTypes     map[string]FieldKind
	maxDepth       int
	maxRegexLen    int
	maxOperations  int
	strictMode     bool
	untrustedInput bool
}

// NewTranslatorConfig creates a TranslatorConfig with default values.
func NewTranslatorConfig() *TranslatorConfig {
	return &TranslatorConfig{
		maxDepth:      DefaultMaxDepth,
		maxRegexLen:   DefaultMaxRegexLength,
		maxOperations: DefaultMaxOperations,
		strictMode:    false,
	}
}

// TranslatorOption configures a translator.
type TranslatorOption func(*TranslatorConfig)

// WithAllowedFields sets a whitelist of allowed field names.
// When set, any field not in the list will cause translation to fail.
// This is a security feature to prevent unauthorized field access.
//
// Example:
//
//	trans := mongo.NewTranslator(filter.WithAllowedFields("name", "age", "email"))
func WithAllowedFields(fields ...string) TranslatorOption {
	return func(c *TranslatorConfig) {
		c.allowedFields = make(map[string]struct{}, len(fields))
		for _, f := range fields {
			c.allowedFields[f] = struct{}{}
		}
	}
}

// WithFieldMapping sets a mapping from CEL field names to database column names.
// This allows using user-friendly names in CEL while mapping to actual DB columns.
//
// Example:
//
//	trans := mongo.NewTranslator(filter.WithFieldMapping(map[string]string{
//	    "userName": "user_name",
//	    "createdAt": "created_at",
//	}))
func WithFieldMapping(mapping map[string]string) TranslatorOption {
	return func(c *TranslatorConfig) {
		c.fieldMapping = mapping
	}
}

// WithFieldTypes declares the expected kind for each queryable field.
// When set, translation fails with [ErrFieldTypeMismatch] for literals whose
// Go type does not match the declared kind. Fields absent from the map skip
// the check. Keys are CEL-side field names, the same as [WithAllowedFields].
//
// Example:
//
//	trans := mongo.NewTranslator(filter.WithFieldTypes(map[string]filter.FieldKind{
//	    "status":    filter.FieldKindInt,
//	    "createdAt": filter.FieldKindTimestamp,
//	}))
func WithFieldTypes(types map[string]FieldKind) TranslatorOption {
	return func(c *TranslatorConfig) {
		c.fieldTypes = types
	}
}

// WithMaxDepth sets the maximum allowed expression nesting depth.
// This is a DoS protection to prevent deeply nested expressions from
// consuming excessive resources.
//
// Example:
//
//	trans := mongo.NewTranslator(filter.WithMaxDepth(10))
func WithMaxDepth(depth int) TranslatorOption {
	return func(c *TranslatorConfig) {
		if depth > 0 {
			c.maxDepth = depth
		}
	}
}

// WithMaxRegexLength sets the maximum allowed length for regex patterns
// in matches(). This prevents excessive CPU usage from complex regex evaluation.
//
// Example:
//
//	eval := filter.NewEvaluator(filter.WithMaxRegexLength(512))
func WithMaxRegexLength(n int) TranslatorOption {
	return func(c *TranslatorConfig) {
		if n > 0 {
			c.maxRegexLen = n
		}
	}
}

// WithMaxOperations sets the maximum number of AST node visits during
// evaluation or translation. This prevents DoS via wide expressions
// (e.g., hundreds of OR-ed conditions at depth 1).
//
// Example:
//
//	eval := filter.NewEvaluator(filter.WithMaxOperations(500))
func WithMaxOperations(n int) TranslatorOption {
	return func(c *TranslatorConfig) {
		if n > 0 {
			c.maxOperations = n
		}
	}
}

// WithStrictMode enables strict mode, which causes translation to fail
// on any unsupported operation rather than ignoring it.
//
// Example:
//
//	trans := mongo.NewTranslator(filter.WithStrictMode(true))
func WithStrictMode(strict bool) TranslatorOption {
	return func(c *TranslatorConfig) {
		c.strictMode = strict
	}
}

// WithUntrustedInput marks the translator/evaluator as receiving CEL
// expressions from external (untrusted) sources, e.g. an end-user query
// parameter. With this flag set, [TranslatorConfig.RequireAllowlist] —
// invoked by every translator's Translate entry point — refuses to proceed
// unless [WithAllowedFields] is also configured. Without an allowlist a
// hostile client can filter on any internal field the storage layer
// happens to index (e.g. `passwordHash > ""` to enumerate accounts), so
// "no allowlist" plus "untrusted input" is treated as a misconfiguration
// rather than a permissive default.
//
// Example:
//
//	trans := mongo.NewTranslator(
//	    filter.WithUntrustedInput(),
//	    filter.WithAllowedFields("name", "status", "createdAt"),
//	)
func WithUntrustedInput() TranslatorOption {
	return func(c *TranslatorConfig) {
		c.untrustedInput = true
	}
}

// ApplyFieldMapping applies the field mapping to a field name.
// Returns the mapped name if found, otherwise returns the original name.
func (c *TranslatorConfig) ApplyFieldMapping(field string) string {
	if c.fieldMapping == nil {
		return field
	}
	if mapped, ok := c.fieldMapping[field]; ok {
		return mapped
	}
	return field
}

// IsFieldAllowed checks if a field is allowed.
// Returns true if no allowlist is set or if the field is in the allowlist.
func (c *TranslatorConfig) IsFieldAllowed(field string) bool {
	if c.allowedFields == nil {
		return true
	}
	_, ok := c.allowedFields[field]
	return ok
}

// FieldKind returns the declared kind for a CEL-side field name, or
// [FieldKindUnspecified] when the field has no declaration.
func (c *TranslatorConfig) FieldKind(field string) FieldKind {
	if c.fieldTypes == nil {
		return FieldKindUnspecified
	}
	return c.fieldTypes[field]
}

// CheckLiteralKind verifies that the right-hand side AST node of a
// comparison or `in` expression carries literal value(s) assignable to
// the kind declared for field. Non-literal right-hand sides (e.g. a
// custom-function call that returned a [BinaryOpNode]) are skipped —
// the check is static and limited to what the parser produced as a
// constant. Fields without a declared kind are accepted unconditionally.
func (c *TranslatorConfig) CheckLiteralKind(field string, right Node) error {
	kind := c.FieldKind(field)
	if kind == FieldKindUnspecified {
		return nil
	}
	switch n := right.(type) {
	case *LiteralNode:
		return checkSingleLiteral(field, kind, n.Value)
	case *ListNode:
		for i, elem := range n.Elements {
			lit, ok := elem.(*LiteralNode)
			if !ok {
				continue
			}
			if lit.Value == nil {
				continue
			}
			if !kindAccepts(kind, lit.Value) {
				return coreerrs.Wrapf(ErrFieldTypeMismatch,
					"field %q (%s): list element [%d] has type %T", field, kind, i, lit.Value)
			}
		}
	}
	return nil
}

// checkSingleLiteral validates one literal value; nil (CEL null) matches anything.
func checkSingleLiteral(field string, kind FieldKind, value any) error {
	if value == nil {
		return nil
	}
	if !kindAccepts(kind, value) {
		return coreerrs.Wrapf(ErrFieldTypeMismatch,
			"field %q (%s): value has type %T", field, kind, value)
	}
	return nil
}

// kindAccepts reports whether value's Go type is assignable to kind.
// FieldKindFloat accepts integer literals too — CEL parses whole numbers
// as int64, so `price >= 100` would otherwise be rejected against a
// double field.
func kindAccepts(kind FieldKind, value any) bool {
	switch kind {
	case FieldKindUnspecified:
		return true
	case FieldKindInt:
		switch value.(type) {
		case int64, uint64:
			return true
		}
	case FieldKindFloat:
		switch value.(type) {
		case float64, int64, uint64:
			return true
		}
	case FieldKindString:
		_, ok := value.(string)
		return ok
	case FieldKindBool:
		_, ok := value.(bool)
		return ok
	case FieldKindBytes:
		_, ok := value.([]byte)
		return ok
	case FieldKindTimestamp:
		_, ok := value.(time.Time)
		return ok
	}
	return false
}

// RequireAllowlist returns [ErrAllowlistRequired] when the config was
// marked as receiving untrusted input ([WithUntrustedInput]) but no
// allowlist was configured ([WithAllowedFields]). Translators and
// evaluators must call this at the entry point of every translation pass
// so misconfiguration surfaces on the first untrusted query rather than
// silently letting the caller filter on arbitrary internal fields.
func (c *TranslatorConfig) RequireAllowlist() error {
	if c.untrustedInput && c.allowedFields == nil {
		return ErrAllowlistRequired
	}
	return nil
}

// MaxDepth returns the configured maximum depth.
func (c *TranslatorConfig) MaxDepth() int {
	return c.maxDepth
}

// MaxRegexLength returns the configured maximum regex pattern length.
func (c *TranslatorConfig) MaxRegexLength() int {
	return c.maxRegexLen
}

// MaxOperations returns the configured maximum number of AST node visits.
func (c *TranslatorConfig) MaxOperations() int {
	return c.maxOperations
}

// StrictMode returns whether strict mode is enabled.
func (c *TranslatorConfig) StrictMode() bool {
	return c.strictMode
}

// SetAllowedFields sets the allowed fields map.
func (c *TranslatorConfig) SetAllowedFields(fields map[string]struct{}) {
	c.allowedFields = fields
}

// SetFieldMapping sets the field mapping.
func (c *TranslatorConfig) SetFieldMapping(mapping map[string]string) {
	c.fieldMapping = mapping
}
