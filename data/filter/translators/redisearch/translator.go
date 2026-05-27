// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisearch

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/altessa-s/go-atlas/data/filter"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// FieldType represents the RediSearch schema type for a field.
type FieldType int

const (
	// FieldTypeNumeric represents a NUMERIC field indexed with SORTABLE.
	FieldTypeNumeric FieldType = iota
	// FieldTypeTag represents a TAG field (exact match, pipe-separated values).
	FieldTypeTag
	// FieldTypeText represents a TEXT field (full-text search with prefix/infix).
	FieldTypeText
)

// tagEscaper replaces RediSearch special characters in TAG values.
var tagEscaper = strings.NewReplacer(
	`,`, `\,`,
	`.`, `\.`,
	`!`, `\!`,
	`{`, `\{`,
	`}`, `\}`,
	`(`, `\(`,
	`)`, `\)`,
	`"`, `\"`,
	`-`, `\-`,
	`@`, `\@`,
	`:`, `\:`,
	`;`, `\;`,
	`[`, `\[`,
	`]`, `\]`,
	`'`, `\'`,
	`|`, `\|`,
	`~`, `\~`,
	`*`, `\*`,
	` `, `\ `,
)

// Translator converts filter AST nodes to RediSearch query strings.
type Translator struct {
	config *filter.TranslatorConfig
	schema map[string]FieldType
	depth  int
}

// NewTranslator creates a new RediSearch translator with the given schema and options.
// The schema maps field names (after field mapping is applied) to their RediSearch types.
func NewTranslator(schema map[string]FieldType, opts ...filter.TranslatorOption) *Translator {
	cfg := filter.NewTranslatorConfig()
	for _, opt := range opts {
		opt(cfg)
	}
	return &Translator{config: cfg, schema: schema}
}

// Translate converts a filter AST node to a RediSearch query string.
// Returns "*" for a nil node (match all).
func (t *Translator) Translate(node filter.Node) (string, error) {
	if err := t.config.RequireAllowlist(); err != nil {
		return "", err
	}
	if node == nil {
		return "*", nil
	}
	t.depth = 0
	result, err := node.Accept(t)
	if err != nil {
		return "", err
	}

	s, ok := result.(string)
	if !ok {
		return "", coreerrs.Wrapf(filter.ErrInvalidExpression, "expected string, got %T", result)
	}
	if s == "" {
		return "*", nil
	}
	return s, nil
}

// VisitLiteral converts a literal value to its string representation.
func (t *Translator) VisitLiteral(n *filter.LiteralNode) (any, error) {
	return t.formatLiteral(n.Value)
}

// VisitIdent converts an identifier to a field reference.
func (t *Translator) VisitIdent(n *filter.IdentNode) (any, error) {
	field := n.Name
	if !t.config.IsFieldAllowed(field) {
		return nil, coreerrs.Wrapf(filter.ErrFieldNotAllowed, "%s", field)
	}
	return t.config.ApplyFieldMapping(field), nil
}

// VisitBinaryOp converts a binary operation to a RediSearch query fragment.
func (t *Translator) VisitBinaryOp(n *filter.BinaryOpNode) (any, error) {
	if err := t.checkDepth(); err != nil {
		return nil, err
	}
	t.depth++
	defer func() { t.depth-- }()

	switch n.Op {
	case filter.OpAnd:
		return t.translateLogicalAnd(n.Left, n.Right)
	case filter.OpOr:
		return t.translateLogicalOr(n.Left, n.Right)
	case filter.OpIn:
		return t.translateIn(n.Left, n.Right)
	default:
		return t.translateComparison(n.Op, n.Left, n.Right)
	}
}

// VisitUnaryOp converts a unary operation to a RediSearch query fragment.
func (t *Translator) VisitUnaryOp(n *filter.UnaryOpNode) (any, error) {
	if err := t.checkDepth(); err != nil {
		return nil, err
	}
	t.depth++
	defer func() { t.depth-- }()

	if n.Op == filter.OpNot {
		return t.translateNot(n.Operand)
	}
	return nil, coreerrs.Wrapf(filter.ErrUnsupportedOperation, "unary operator %v", n.Op)
}

// VisitCall converts a function call to a RediSearch query fragment.
func (t *Translator) VisitCall(n *filter.CallNode) (any, error) {
	if err := t.checkDepth(); err != nil {
		return nil, err
	}
	t.depth++
	defer func() { t.depth-- }()

	switch n.Op {
	case filter.OpContains:
		return t.translateContains(n.Target, n.Args)
	case filter.OpStartsWith:
		return t.translateStartsWith(n.Target, n.Args)
	case filter.OpEndsWith:
		return nil, coreerrs.Wrap(filter.ErrUnsupportedOperation, "endsWith is not supported by RediSearch")
	case filter.OpMatches:
		return nil, coreerrs.Wrap(filter.ErrUnsupportedOperation, "matches is not supported by RediSearch")
	case filter.OpSize:
		return nil, coreerrs.Wrap(filter.ErrUnsupportedOperation, "size is not supported by RediSearch")
	case filter.OpHas, filter.OpExists:
		return nil, coreerrs.Wrap(filter.ErrUnsupportedOperation, "has/exists is not supported by RediSearch")
	default:
		return nil, coreerrs.Wrapf(filter.ErrUnsupportedOperation, "function %v", n.Op)
	}
}

// VisitList converts a list to a slice of values.
func (t *Translator) VisitList(n *filter.ListNode) (any, error) {
	result := make([]any, 0, len(n.Elements))
	for _, elem := range n.Elements {
		val, err := elem.Accept(t)
		if err != nil {
			return nil, err
		}
		result = append(result, val)
	}
	return result, nil
}

// translateComparison handles comparison operators (==, !=, <, >, <=, >=).
func (t *Translator) translateComparison(op filter.Operator, left, right filter.Node) (string, error) {
	field, err := t.getFieldName(left)
	if err != nil {
		return "", err
	}

	if ident, ok := left.(*filter.IdentNode); ok {
		if err = t.config.CheckLiteralKind(ident.Name, right); err != nil {
			return "", err
		}
	}

	value, err := right.Accept(t)
	if err != nil {
		return "", err
	}

	ft := t.fieldType(field)
	return t.buildComparison(field, op, value, ft)
}

// buildComparison creates a RediSearch comparison expression.
func (t *Translator) buildComparison(field string, op filter.Operator, value any, ft FieldType) (string, error) {
	switch ft {
	case FieldTypeNumeric:
		return t.buildNumericComparison(field, op, value)
	case FieldTypeTag:
		return t.buildTagComparison(field, op, value)
	case FieldTypeText:
		return t.buildTextComparison(field, op, value)
	default:
		return "", coreerrs.Wrapf(filter.ErrInvalidExpression, "unknown field type for %q", field)
	}
}

// buildNumericComparison creates a RediSearch NUMERIC range expression.
func (t *Translator) buildNumericComparison(field string, op filter.Operator, value any) (string, error) {
	v := t.formatNumericValue(value)

	switch op {
	case filter.OpEqual:
		return fmt.Sprintf("@%s:[%s %s]", field, v, v), nil
	case filter.OpNotEqual:
		return fmt.Sprintf("-@%s:[%s %s]", field, v, v), nil
	case filter.OpLT:
		return fmt.Sprintf("@%s:[-inf (%s]", field, v), nil
	case filter.OpLTE:
		return fmt.Sprintf("@%s:[-inf %s]", field, v), nil
	case filter.OpGT:
		return fmt.Sprintf("@%s:[(%s +inf]", field, v), nil
	case filter.OpGTE:
		return fmt.Sprintf("@%s:[%s +inf]", field, v), nil
	default:
		return "", coreerrs.Wrapf(filter.ErrUnsupportedOperation, "numeric comparison with operator %v", op)
	}
}

// buildTagComparison creates a RediSearch TAG expression.
func (t *Translator) buildTagComparison(field string, op filter.Operator, value any) (string, error) {
	v := t.escapeTagValue(fmt.Sprintf("%v", value))

	switch op {
	case filter.OpEqual:
		return fmt.Sprintf("@%s:{%s}", field, v), nil
	case filter.OpNotEqual:
		return fmt.Sprintf("-@%s:{%s}", field, v), nil
	default:
		return "", coreerrs.Wrapf(filter.ErrUnsupportedOperation, "TAG field %q does not support operator %v", field, op)
	}
}

// buildTextComparison creates a RediSearch TEXT expression (== only for exact match via TAG fallback).
func (t *Translator) buildTextComparison(field string, op filter.Operator, value any) (string, error) {
	v := fmt.Sprintf("%v", value)

	switch op {
	case filter.OpEqual:
		return fmt.Sprintf("@%s:(%s)", field, v), nil
	case filter.OpNotEqual:
		return fmt.Sprintf("-@%s:(%s)", field, v), nil
	default:
		return "", coreerrs.Wrapf(filter.ErrUnsupportedOperation, "TEXT field %q does not support operator %v", field, op)
	}
}

// translateLogicalAnd handles the && operator. RediSearch uses space for AND.
func (t *Translator) translateLogicalAnd(left, right filter.Node) (string, error) {
	l, err := t.getQueryString(left)
	if err != nil {
		return "", err
	}
	r, err := t.getQueryString(right)
	if err != nil {
		return "", err
	}
	return "(" + l + " " + r + ")", nil
}

// translateLogicalOr handles the || operator. RediSearch uses | for OR.
func (t *Translator) translateLogicalOr(left, right filter.Node) (string, error) {
	l, err := t.getQueryString(left)
	if err != nil {
		return "", err
	}
	r, err := t.getQueryString(right)
	if err != nil {
		return "", err
	}
	return "(" + l + ")|(" + r + ")", nil
}

// translateNot handles the ! operator.
func (t *Translator) translateNot(operand filter.Node) (string, error) {
	// For simple identifiers (like !active), treat as field == false TAG
	if ident, ok := operand.(*filter.IdentNode); ok {
		field := ident.Name
		if !t.config.IsFieldAllowed(field) {
			return "", coreerrs.Wrapf(filter.ErrFieldNotAllowed, "%s", field)
		}
		mapped := t.config.ApplyFieldMapping(field)
		return fmt.Sprintf("-@%s:{true}", mapped), nil
	}

	inner, err := t.getQueryString(operand)
	if err != nil {
		return "", err
	}
	return "-(" + inner + ")", nil
}

// translateIn handles the in operator for TAG fields.
func (t *Translator) translateIn(left, right filter.Node) (string, error) {
	field, err := t.getFieldName(left)
	if err != nil {
		return "", err
	}

	if ident, ok := left.(*filter.IdentNode); ok {
		if err = t.config.CheckLiteralKind(ident.Name, right); err != nil {
			return "", err
		}
	}

	values, err := right.Accept(t)
	if err != nil {
		return "", err
	}

	valuesSlice, ok := values.([]any)
	if !ok {
		return "", coreerrs.Wrapf(filter.ErrInvalidExpression, "in operator requires a list, got %T", values)
	}

	escaped := make([]string, 0, len(valuesSlice))
	for _, v := range valuesSlice {
		escaped = append(escaped, t.escapeTagValue(fmt.Sprintf("%v", v)))
	}

	return fmt.Sprintf("@%s:{%s}", field, strings.Join(escaped, "|")), nil
}

// translateContains handles field.contains("sub") for TEXT fields → @field:*sub*.
func (t *Translator) translateContains(target filter.Node, args []filter.Node) (string, error) {
	field, err := t.getFieldName(target)
	if err != nil {
		return "", err
	}

	if len(args) != 1 {
		return "", coreerrs.Wrap(filter.ErrInvalidExpression, "contains requires exactly 1 argument")
	}

	arg, err := args[0].Accept(t)
	if err != nil {
		return "", err
	}

	s, ok := arg.(string)
	if !ok {
		return "", coreerrs.Wrap(filter.ErrInvalidExpression, "contains argument must be a string")
	}

	return fmt.Sprintf("@%s:*%s*", field, s), nil
}

// translateStartsWith handles field.startsWith("pre") for TEXT fields → @field:pre*.
func (t *Translator) translateStartsWith(target filter.Node, args []filter.Node) (string, error) {
	field, err := t.getFieldName(target)
	if err != nil {
		return "", err
	}

	if len(args) != 1 {
		return "", coreerrs.Wrap(filter.ErrInvalidExpression, "startsWith requires exactly 1 argument")
	}

	arg, err := args[0].Accept(t)
	if err != nil {
		return "", err
	}

	s, ok := arg.(string)
	if !ok {
		return "", coreerrs.Wrap(filter.ErrInvalidExpression, "startsWith argument must be a string")
	}

	return fmt.Sprintf("@%s:%s*", field, s), nil
}

// getFieldName extracts the field name from a node.
func (t *Translator) getFieldName(node filter.Node) (string, error) {
	result, err := node.Accept(t)
	if err != nil {
		return "", err
	}
	field, ok := result.(string)
	if !ok {
		return "", coreerrs.Wrapf(filter.ErrInvalidExpression, "expected field name, got %T", result)
	}
	return field, nil
}

// getQueryString accepts a node and ensures the result is a string.
func (t *Translator) getQueryString(node filter.Node) (string, error) {
	result, err := node.Accept(t)
	if err != nil {
		return "", err
	}
	s, ok := result.(string)
	if !ok {
		return "", coreerrs.Wrapf(filter.ErrInvalidExpression, "expected query string, got %T", result)
	}
	return s, nil
}

// fieldType returns the FieldType for a given field name.
// Defaults to FieldTypeTag if not found in schema.
func (t *Translator) fieldType(field string) FieldType {
	if ft, ok := t.schema[field]; ok {
		return ft
	}
	return FieldTypeTag
}

// formatLiteral converts a Go value to a string representation for RediSearch.
func (t *Translator) formatLiteral(v any) (any, error) {
	switch val := v.(type) {
	case nil:
		return "null", nil
	case bool:
		if val {
			return "true", nil
		}
		return "false", nil
	case int64:
		return strconv.FormatInt(val, 10), nil
	case uint64:
		return strconv.FormatUint(val, 10), nil
	case float64:
		return strconv.FormatFloat(val, 'g', -1, 64), nil
	case string:
		return val, nil
	default:
		return nil, coreerrs.Wrapf(filter.ErrUnsupportedType, "%T", v)
	}
}

// formatNumericValue formats a value for RediSearch numeric range syntax.
func (t *Translator) formatNumericValue(value any) string {
	switch v := value.(type) {
	case int64:
		return strconv.FormatInt(v, 10)
	case uint64:
		return strconv.FormatUint(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64)
	default:
		return fmt.Sprintf("%v", value)
	}
}

// escapeTagValue escapes RediSearch special characters in TAG values.
func (t *Translator) escapeTagValue(s string) string {
	return tagEscaper.Replace(s)
}

// EscapeTag is a public helper that escapes RediSearch special characters in TAG values.
// Useful for building raw RediSearch queries outside the translator.
func EscapeTag(s string) string {
	return tagEscaper.Replace(s)
}

// checkDepth verifies we haven't exceeded maximum nesting depth.
func (t *Translator) checkDepth() error {
	if t.depth >= t.config.MaxDepth() {
		return coreerrs.Wrapf(filter.ErrMaxDepthExceeded, "depth %d exceeds maximum %d", t.depth, t.config.MaxDepth())
	}
	return nil
}

// Ensure Translator implements filter.Visitor
var _ filter.Visitor = (*Translator)(nil)
