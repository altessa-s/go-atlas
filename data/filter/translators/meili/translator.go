// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package meili

import (
	"strconv"
	"strings"
	"time"

	"github.com/altessa-s/go-atlas/data/filter"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Translator converts filter AST nodes to Meilisearch filter expressions.
type Translator struct {
	config *filter.TranslatorConfig
	depth  int
}

// NewTranslator creates a new Meilisearch translator with the given options.
func NewTranslator(opts ...filter.TranslatorOption) *Translator {
	cfg := filter.NewTranslatorConfig()
	for _, opt := range opts {
		opt(cfg)
	}
	return &Translator{config: cfg}
}

// Translate converts a filter AST node to a Meilisearch filter expression.
func (t *Translator) Translate(node filter.Node) (string, error) {
	if err := t.config.RequireAllowlist(); err != nil {
		return "", err
	}
	t.depth = 0
	result, err := node.Accept(t)
	if err != nil {
		return "", err
	}

	s, ok := result.(string)
	if !ok {
		return "", coreerrs.Wrapf(filter.ErrInvalidExpression, "expected filter expression, got %T", result)
	}
	return s, nil
}

// VisitLiteral converts a literal value to its Meilisearch representation.
func (t *Translator) VisitLiteral(n *filter.LiteralNode) (any, error) {
	switch n.Value.(type) {
	case nil, bool, int64, uint64, float64, string, []byte, time.Time:
		return n.Value, nil
	default:
		return nil, coreerrs.Wrapf(filter.ErrUnsupportedType, "%T", n.Value)
	}
}

// VisitIdent converts an identifier to a field reference.
func (t *Translator) VisitIdent(n *filter.IdentNode) (any, error) {
	field := n.Name
	if !t.config.IsFieldAllowed(field) {
		return nil, coreerrs.Wrapf(filter.ErrFieldNotAllowed, "%s", field)
	}
	return t.config.ApplyFieldMapping(field), nil
}

// VisitBinaryOp converts a binary operation to a Meilisearch filter.
func (t *Translator) VisitBinaryOp(n *filter.BinaryOpNode) (any, error) {
	if err := t.checkDepth(); err != nil {
		return nil, err
	}
	t.depth++
	defer func() { t.depth-- }()

	switch n.Op {
	case filter.OpAnd:
		return t.translateLogical("AND", n.Left, n.Right)
	case filter.OpOr:
		return t.translateLogical("OR", n.Left, n.Right)
	case filter.OpIn:
		return t.translateIn(n.Left, n.Right)
	default:
		return t.translateComparison(n.Op, n.Left, n.Right)
	}
}

// VisitUnaryOp converts a unary operation to a Meilisearch filter.
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

// VisitCall converts a function call to a Meilisearch filter.
func (t *Translator) VisitCall(n *filter.CallNode) (any, error) {
	if err := t.checkDepth(); err != nil {
		return nil, err
	}
	t.depth++
	defer func() { t.depth-- }()

	switch n.Op {
	case filter.OpContains:
		return t.translateStringFunc(n.Target, n.Args, "CONTAINS")
	case filter.OpStartsWith:
		return t.translateStringFunc(n.Target, n.Args, "STARTS WITH")
	case filter.OpHas, filter.OpExists:
		return t.translateExists(n.Target)
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

// translateComparison handles comparison operators.
func (t *Translator) translateComparison(op filter.Operator, left, right filter.Node) (string, error) {
	if isSizeCall(left) || isSizeCall(right) {
		return "", coreerrs.Wrap(filter.ErrUnsupportedOperation, "size() comparisons")
	}

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

	return t.buildComparisonFilter(field, op, value)
}

// buildComparisonFilter creates a Meilisearch comparison filter.
func (t *Translator) buildComparisonFilter(field string, op filter.Operator, value any) (string, error) {
	if value == nil {
		switch op {
		case filter.OpEqual:
			return field + " IS NULL", nil
		case filter.OpNotEqual:
			return field + " IS NOT NULL", nil
		default:
			return "", coreerrs.Wrapf(filter.ErrUnsupportedOperation, "comparison %v with null", op)
		}
	}

	formatted, err := formatLiteral(value)
	if err != nil {
		return "", err
	}

	var meiliOp string
	switch op {
	case filter.OpEqual:
		meiliOp = "="
	case filter.OpNotEqual:
		meiliOp = "!="
	case filter.OpLT:
		meiliOp = "<"
	case filter.OpLTE:
		meiliOp = "<="
	case filter.OpGT:
		meiliOp = ">"
	case filter.OpGTE:
		meiliOp = ">="
	default:
		return "", coreerrs.Wrapf(filter.ErrUnsupportedOperation, "comparison operator %v", op)
	}

	return field + " " + meiliOp + " " + formatted, nil
}

// translateLogical handles && and || operators.
func (t *Translator) translateLogical(meiliOp string, left, right filter.Node) (string, error) {
	leftStr, err := t.acceptString(left)
	if err != nil {
		return "", err
	}
	rightStr, err := t.acceptString(right)
	if err != nil {
		return "", err
	}
	return "(" + leftStr + ") " + meiliOp + " (" + rightStr + ")", nil
}

// translateNot handles the ! operator.
func (t *Translator) translateNot(operand filter.Node) (string, error) {
	if _, ok := operand.(*filter.IdentNode); ok {
		field, err := t.getFieldName(operand)
		if err != nil {
			return "", err
		}
		return "NOT (" + field + " = true)", nil
	}

	inner, err := t.acceptString(operand)
	if err != nil {
		return "", err
	}
	return "NOT (" + inner + ")", nil
}

// translateIn handles the in operator.
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

	formatted := make([]string, 0, len(valuesSlice))
	for _, v := range valuesSlice {
		s, err := formatLiteral(v)
		if err != nil {
			return "", err
		}
		formatted = append(formatted, s)
	}

	return field + " IN [" + strings.Join(formatted, ", ") + "]", nil
}

// translateStringFunc handles single-argument string predicates: contains, startsWith.
func (t *Translator) translateStringFunc(target filter.Node, args []filter.Node, meiliOp string) (string, error) {
	field, err := t.getFieldName(target)
	if err != nil {
		return "", err
	}

	if len(args) != 1 {
		return "", coreerrs.Wrap(filter.ErrInvalidExpression, "string function requires exactly 1 argument")
	}

	arg, err := args[0].Accept(t)
	if err != nil {
		return "", err
	}

	pattern, ok := arg.(string)
	if !ok {
		return "", coreerrs.Wrap(filter.ErrInvalidExpression, "string function argument must be a string")
	}

	return field + " " + meiliOp + " " + quoteString(pattern), nil
}

// translateExists handles the has() / exists function.
func (t *Translator) translateExists(target filter.Node) (string, error) {
	field, err := t.getFieldName(target)
	if err != nil {
		return "", err
	}
	return field + " EXISTS", nil
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

// acceptString visits a node expected to produce a filter clause string.
// A bare identifier is treated as a boolean equality (`field = true`).
func (t *Translator) acceptString(node filter.Node) (string, error) {
	if ident, ok := node.(*filter.IdentNode); ok {
		field, err := t.getFieldName(ident)
		if err != nil {
			return "", err
		}
		return field + " = true", nil
	}

	result, err := node.Accept(t)
	if err != nil {
		return "", err
	}
	s, ok := result.(string)
	if !ok {
		return "", coreerrs.Wrapf(filter.ErrInvalidExpression, "expected filter clause, got %T", result)
	}
	return s, nil
}

// checkDepth verifies we haven't exceeded maximum nesting depth.
func (t *Translator) checkDepth() error {
	if t.depth >= t.config.MaxDepth() {
		return coreerrs.Wrapf(filter.ErrMaxDepthExceeded, "depth %d exceeds maximum %d", t.depth, t.config.MaxDepth())
	}
	return nil
}

// formatLiteral renders a Go value as a Meilisearch filter literal.
func formatLiteral(v any) (string, error) {
	switch val := v.(type) {
	case nil:
		return "NULL", nil
	case bool:
		if val {
			return "true", nil
		}
		return "false", nil
	case string:
		return quoteString(val), nil
	case []byte:
		return quoteString(string(val)), nil
	case int64:
		return strconv.FormatInt(val, 10), nil
	case uint64:
		return strconv.FormatUint(val, 10), nil
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64), nil
	case time.Time:
		return strconv.FormatInt(val.Unix(), 10), nil
	default:
		return "", coreerrs.Wrapf(filter.ErrUnsupportedType, "%T", v)
	}
}

// isSizeCall reports whether n is a `size(x)` / `x.size()` call.
func isSizeCall(n filter.Node) bool {
	call, ok := n.(*filter.CallNode)
	return ok && call.Op == filter.OpSize
}

// quoteString wraps a string in double quotes, escaping `\` and `"`.
func quoteString(s string) string {
	const surroundingQuotes = 2 // opening + closing `"`

	var b strings.Builder
	b.Grow(len(s) + surroundingQuotes)
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// Ensure Translator implements filter.Visitor.
var _ filter.Visitor = (*Translator)(nil)
