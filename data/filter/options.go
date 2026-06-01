// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter

import (
	"fmt"
	"time"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=translatorOptions --output=translator_options_gen.go --option-type=TranslatorOption

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

// translatorOptions is the optgen target — a private struct that holds
// the configuration shared by every database translator and the
// in-memory evaluator. The struct is unexported so callers cannot
// bypass the constructor; cross-package access happens through the
// public [TranslatorContext] type alias and the read methods defined on
// *translatorOptions below.
//
// allowedFields, fieldMapping, and fieldTypes are stored as
// [coremaps.ImmutableMap] — they are built once by [WithAllowedFields]
// / [WithFieldMapping] / [WithFieldTypes] and only read afterwards,
// which is precisely the build-once-read-many shape AGENTS.md mandates
// ImmutableMap for. All three setters are hand-written because optgen
// cannot express the variadic / map-to-ImmutableMap conversions.
type translatorOptions struct {
	allowedFields  *coremaps.ImmutableMap[string, struct{}]  `opt:"-"`
	fieldMapping   *coremaps.ImmutableMap[string, string]    `opt:"-"`
	fieldTypes     *coremaps.ImmutableMap[string, FieldKind] `opt:"-"`
	maxDepth       int                                       `optgen:"default=DefaultMaxDepth" optval:"positive"`
	maxRegexLength int                                       `optgen:"default=DefaultMaxRegexLength" optval:"positive"`
	maxOperations  int                                       `optgen:"default=DefaultMaxOperations" optval:"positive"`
	untrustedInput bool
}

// TranslatorContext is the public read-side handle that translator
// subpackages and the in-memory evaluator thread through their
// Translate / Evaluate calls. It is a type alias for the
// package-private [translatorOptions] struct, so the read methods are
// accessible cross-package while the raw fields stay encapsulated. A
// *TranslatorContext is always the product of a successful
// [NewTranslatorContext] call — there is no allow-list misconfiguration
// left to surface at translation time.
type TranslatorContext = translatorOptions

// NewTranslatorContext builds a [TranslatorContext] with the given
// options applied and validates the result. It fails with
// [ErrAllowlistRequired] when [WithUntrustedInput] was set but no
// non-empty [WithAllowedFields] allow-list was provided — that
// combination would otherwise silently produce a deny-all translator,
// which is almost always a configuration bug.
//
// Translator constructors call this from their NewTranslator
// implementations and propagate the error. Misconfiguration therefore
// surfaces at process start rather than on the first request.
func NewTranslatorContext(opts ...TranslatorOption) (*TranslatorContext, error) {
	ctx := newTranslatorOptions(opts...)
	if err := ctx.requireAllowlist(); err != nil {
		return nil, err
	}
	return ctx, nil
}

// WithAllowedFields sets a whitelist of allowed field names. When set,
// any field not in the list will cause translation or evaluation to
// fail with [ErrFieldNotAllowed]. Keys are matched against the
// CEL-side field name verbatim, so callers configure nested fields
// exactly as they appear in the DSL ("address.city"). The list is
// frozen into a [coremaps.ImmutableMap] so lookups are
// allocation-free and the policy cannot drift after the context is
// constructed.
//
// Example:
//
//	trans, err := mongo.NewTranslator(filter.WithAllowedFields("name", "age", "email"))
func WithAllowedFields(fields ...string) TranslatorOption {
	return func(o *translatorOptions) {
		m := make(map[string]struct{}, len(fields))
		for _, f := range fields {
			m[f] = struct{}{}
		}
		o.allowedFields = coremaps.NewImmutableMap(m)
	}
}

// WithFieldMapping sets a mapping from CEL field names to database
// column names. This lets callers expose user-friendly names in the
// API surface while filtering against the underlying storage
// representation. The mapping is copied into a
// [coremaps.ImmutableMap] so subsequent mutations of the caller's map
// cannot affect a constructed translator.
//
// Example:
//
//	trans, err := mongo.NewTranslator(filter.WithFieldMapping(map[string]string{
//	    "userName": "user_name",
//	    "createdAt": "created_at",
//	}))
func WithFieldMapping(mapping map[string]string) TranslatorOption {
	return func(o *translatorOptions) {
		o.fieldMapping = coremaps.NewImmutableMap(mapping)
	}
}

// WithFieldTypes declares the expected kind for each queryable field.
// When set, translation or evaluation fails with
// [ErrFieldTypeMismatch] for literals whose Go type does not match the
// declared kind. Fields absent from the map skip the check. Keys are
// CEL-side field names, the same as [WithAllowedFields].
//
// Example:
//
//	trans, err := mongo.NewTranslator(filter.WithFieldTypes(map[string]filter.FieldKind{
//	    "status":    filter.FieldKindInt,
//	    "createdAt": filter.FieldKindTimestamp,
//	}))
func WithFieldTypes(types map[string]FieldKind) TranslatorOption {
	return func(o *translatorOptions) {
		o.fieldTypes = coremaps.NewImmutableMap(types)
	}
}

// ApplyFieldMapping applies the field mapping to a field name. Returns
// the mapped name if found, otherwise returns the original name.
func (o *translatorOptions) ApplyFieldMapping(field string) string {
	if o.fieldMapping == nil {
		return field
	}
	if mapped, ok := o.fieldMapping.Get(field); ok {
		return mapped
	}
	return field
}

// IsFieldAllowed reports whether a field is allowed. Returns true when
// no allow-list is configured at all.
func (o *translatorOptions) IsFieldAllowed(field string) bool {
	if o.allowedFields == nil {
		return true
	}
	return o.allowedFields.Contains(field)
}

// FieldKind returns the declared kind for a CEL-side field name, or
// [FieldKindUnspecified] when the field has no declaration.
func (o *translatorOptions) FieldKind(field string) FieldKind {
	if o.fieldTypes == nil {
		return FieldKindUnspecified
	}
	kind, _ := o.fieldTypes.Get(field)
	return kind
}

// CheckLiteralKind verifies that the right-hand side AST node of a
// comparison or `in` expression carries literal value(s) assignable to
// the kind declared for field. Non-literal right-hand sides (e.g. a
// custom-function call that returned a [BinaryOpNode]) are skipped —
// the check is static and limited to what the parser produced as a
// constant. Fields without a declared kind are accepted unconditionally.
//
// CheckLiteralKind operates on one (field, literal) pair. For a generic
// comparison whose operand order is not known up-front, prefer
// [TranslatorContext.CheckComparison] — it inspects both sides and
// dispatches to CheckLiteralKind for whichever one is the field.
func (o *translatorOptions) CheckLiteralKind(field string, right Node) error {
	kind := o.FieldKind(field)
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
					"field %q (%s): list element [%d] is %s (want %s)",
					field, kind, i, valueKindName(lit.Value), kind)
			}
		}
	}
	return nil
}

// CheckComparison verifies a binary comparison operator against the
// declared field-type schema regardless of operand order. Both
// `status == "x"` and `"x" == status` flow through the same check —
// the symmetric form is required because CEL comparisons are
// commutative in their semantic intent and operators may reorder
// before evaluation.
//
// When neither operand is an IdentNode (literal-on-literal, or
// function-call results on both sides), CheckComparison is a no-op:
// there is no schema-bound field to validate against.
func (o *translatorOptions) CheckComparison(left, right Node) error {
	if ident, ok := left.(*IdentNode); ok {
		return o.CheckLiteralKind(ident.Name, right)
	}
	if ident, ok := right.(*IdentNode); ok {
		return o.CheckLiteralKind(ident.Name, left)
	}
	return nil
}

// MaxDepth returns the configured maximum depth.
func (o *translatorOptions) MaxDepth() int { return o.maxDepth }

// MaxRegexLength returns the configured maximum regex pattern length.
func (o *translatorOptions) MaxRegexLength() int { return o.maxRegexLength }

// MaxOperations returns the configured maximum number of AST node visits.
func (o *translatorOptions) MaxOperations() int { return o.maxOperations }

// requireAllowlist returns [ErrAllowlistRequired] when the context was
// marked as receiving untrusted input ([WithUntrustedInput]) but no
// usable allow-list was configured ([WithAllowedFields]).
// [NewTranslatorContext] runs this once at construction so the check
// disappears from the translation hot path — callers never see a
// misconfigured *TranslatorContext.
//
// An empty allow-list (e.g. `WithAllowedFields()` or
// `WithAllowedFields(slice...)` where the slice happened to be empty)
// is treated the same as no allow-list at all: the deny-all behavior
// it would otherwise produce is almost always a configuration bug — a
// forgotten append, an empty slice from a config loader, a misspelled
// struct field — and it is more useful to fail loudly than to silently
// reject every query.
func (o *translatorOptions) requireAllowlist() error {
	if !o.untrustedInput {
		return nil
	}
	if o.allowedFields == nil || o.allowedFields.Len() == 0 {
		return ErrAllowlistRequired
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
			"field %q (%s): value is %s (want %s)",
			field, kind, valueKindName(value), kind)
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
		// Unreachable in practice — every caller short-circuits on
		// Unspecified before reaching kindAccepts. Kept as an explicit
		// case so the exhaustive-switch linter does not flag the
		// switch, and so a future caller that bypasses the short-circuit
		// inherits the safe accept-all behavior.
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

// valueKindName returns the [FieldKind]-flavored label for a Go value,
// so error messages speak in DSL terms (int / string / bool / …)
// instead of Go's typename (int64 / []byte / time.Time). Falls back to
// %T-style formatting for values that don't map to any FieldKind —
// the message stays informative even for unexpected types.
func valueKindName(value any) string {
	switch value.(type) {
	case int64, uint64:
		return FieldKindInt.String()
	case float64:
		return FieldKindFloat.String()
	case string:
		return FieldKindString.String()
	case bool:
		return FieldKindBool.String()
	case []byte:
		return FieldKindBytes.String()
	case time.Time:
		return FieldKindTimestamp.String()
	default:
		return fmt.Sprintf("%T", value)
	}
}
