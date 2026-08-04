// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lua

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/altessa-s/go-atlas/data/filter"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

const (
	luaTrue  = "true"
	luaFalse = "false"
)

// luaStringEscaper replaces special characters in Lua string literals.
var luaStringEscaper = strings.NewReplacer(
	`\`, `\\`,
	`"`, `\"`,
	"\n", `\n`,
	"\r", `\r`,
	"\t", `\t`,
	"\x00", `\0`,
)

// Translator converts filter AST nodes to Lua boolean expressions.
type Translator struct {
	config   *filter.TranslatorContext
	tableVar string
	depth    filter.DepthGuard
}

// NewTranslator creates a new Lua translator with the given table
// variable name and options. If tableVar is empty, "d" is used as the
// default. Returns [filter.ErrAllowlistRequired] when
// [filter.WithUntrustedInput] is set without a non-empty
// [filter.WithAllowedFields] — the misconfiguration is surfaced here
// rather than on the first Translate call.
func NewTranslator(tableVar string, opts ...filter.TranslatorOption) (*Translator, error) {
	if tableVar == "" {
		tableVar = "d"
	}
	ctx, err := filter.NewTranslatorContext(opts...)
	if err != nil {
		return nil, err
	}
	return &Translator{config: ctx, tableVar: tableVar, depth: filter.NewDepthGuard(ctx.MaxDepth())}, nil
}

// Translate converts a filter AST node to a Lua boolean expression string.
// Returns "true" for a nil node (match all).
func (t *Translator) Translate(node filter.Node) (string, error) {
	if node == nil {
		return luaTrue, nil
	}
	t.depth.Reset()
	result, err := node.Accept(t)
	if err != nil {
		return "", err
	}

	s, ok := result.(string)
	if !ok {
		return "", coreerrs.Wrapf(filter.ErrInvalidExpression, "expected string, got %T", result)
	}
	return s, nil
}

// VisitLiteral converts a literal value to its Lua representation.
func (t *Translator) VisitLiteral(n *filter.LiteralNode) (any, error) {
	return t.formatLiteral(n.Value)
}

// VisitIdent converts an identifier to a field reference string.
func (t *Translator) VisitIdent(n *filter.IdentNode) (any, error) {
	field := n.Name
	if !t.config.IsFieldAllowed(field) {
		return nil, coreerrs.Wrapf(filter.ErrFieldNotAllowed, "%s", field)
	}
	return t.config.ApplyFieldMapping(field), nil
}

// VisitBinaryOp converts a binary operation to a Lua expression.
func (t *Translator) VisitBinaryOp(n *filter.BinaryOpNode) (any, error) {
	if err := t.depth.Enter(); err != nil {
		return nil, err
	}
	defer t.depth.Leave()

	switch n.Op {
	case filter.OpAnd:
		return t.translateLogical("and", n.Left, n.Right)
	case filter.OpOr:
		return t.translateLogical("or", n.Left, n.Right)
	case filter.OpIn:
		return t.translateIn(n.Left, n.Right)
	default:
		return t.translateComparison(n.Op, n.Left, n.Right)
	}
}

// VisitUnaryOp converts a unary operation to a Lua expression.
func (t *Translator) VisitUnaryOp(n *filter.UnaryOpNode) (any, error) {
	if err := t.depth.Enter(); err != nil {
		return nil, err
	}
	defer t.depth.Leave()

	if n.Op == filter.OpNot {
		return t.translateNot(n.Operand)
	}
	return nil, coreerrs.Wrapf(filter.ErrUnsupportedOperation, "unary operator %v", n.Op)
}

// VisitCall converts a function call to a Lua expression.
func (t *Translator) VisitCall(n *filter.CallNode) (any, error) {
	if err := t.depth.Enter(); err != nil {
		return nil, err
	}
	defer t.depth.Leave()

	switch n.Op {
	case filter.OpContains:
		return t.translateContains(n.Target, n.Args)
	case filter.OpStartsWith:
		return t.translateStartsWith(n.Target, n.Args)
	case filter.OpEndsWith:
		return t.translateEndsWith(n.Target, n.Args)
	case filter.OpMatches:
		return nil, coreerrs.Wrap(filter.ErrUnsupportedOperation, "matches is not supported by Lua translator")
	case filter.OpSize:
		return t.translateSize(n.Target)
	case filter.OpHas, filter.OpExists:
		return t.translateHas(n.Target)
	default:
		return nil, coreerrs.Wrapf(filter.ErrUnsupportedOperation, "function %v", n.Op)
	}
}

// VisitList converts a list to a slice of values.
func (t *Translator) VisitList(n *filter.ListNode) (any, error) {
	return filter.VisitElements(t, n)
}

// fieldRef builds a Lua table access expression from a dotted field name.
// For example, "address.city" with tableVar "d" becomes `d["address"]["city"]`.
func (t *Translator) fieldRef(field string) string {
	parts := strings.Split(field, ".")
	var b strings.Builder
	b.WriteString(t.tableVar)
	for _, p := range parts {
		b.WriteString(`["`)
		b.WriteString(p)
		b.WriteString(`"]`)
	}
	return b.String()
}

// luaOperator maps a filter operator to a Lua comparison operator string.
func (t *Translator) luaOperator(op filter.Operator) (string, error) {
	switch op {
	case filter.OpEqual:
		return "==", nil
	case filter.OpNotEqual:
		return "~=", nil
	case filter.OpLT:
		return "<", nil
	case filter.OpLTE:
		return "<=", nil
	case filter.OpGT:
		return ">", nil
	case filter.OpGTE:
		return ">=", nil
	default:
		return "", coreerrs.Wrapf(filter.ErrUnsupportedOperation, "comparison operator %v", op)
	}
}

// formatLiteral converts a Go value to a Lua literal string.
func (t *Translator) formatLiteral(v any) (string, error) {
	switch val := v.(type) {
	case nil:
		return "nil", nil
	case bool:
		if val {
			return luaTrue, nil
		}
		return luaFalse, nil
	case int64:
		return strconv.FormatInt(val, 10), nil
	case uint64:
		return strconv.FormatUint(val, 10), nil
	case float64:
		return strconv.FormatFloat(val, 'g', -1, 64), nil
	case string:
		return `"` + escapeLuaString(val) + `"`, nil
	default:
		return "", coreerrs.Wrapf(filter.ErrUnsupportedType, "%T", v)
	}
}

// escapeLuaString escapes special characters in a Lua string.
func escapeLuaString(s string) string {
	return luaStringEscaper.Replace(s)
}

// getFieldName extracts the field name from a node via Accept.
func (t *Translator) getFieldName(node filter.Node) (string, error) {
	result, err := node.Accept(t)
	if err != nil {
		return "", err
	}

	// Handle size marker
	if s, ok := result.(sizeMarker); ok {
		return string(s), nil
	}

	field, ok := result.(string)
	if !ok {
		return "", coreerrs.Wrapf(filter.ErrInvalidExpression, "expected field name, got %T", result)
	}
	return field, nil
}

// getStringArg extracts a raw string value from a LiteralNode argument.
func (t *Translator) getStringArg(args []filter.Node) (string, error) {
	if len(args) != 1 {
		return "", coreerrs.Wrap(filter.ErrInvalidExpression, "string function requires exactly 1 argument")
	}

	lit, ok := args[0].(*filter.LiteralNode)
	if !ok {
		return "", coreerrs.Wrap(filter.ErrInvalidExpression, "string function argument must be a literal")
	}
	s, ok := lit.Value.(string)
	if !ok {
		return "", coreerrs.Wrap(filter.ErrInvalidExpression, "string function argument must be a string")
	}
	return s, nil
}

// translateComparison handles comparison operators (==, !=, <, >, <=, >=).
func (t *Translator) translateComparison(op filter.Operator, left, right filter.Node) (string, error) {
	// Check if left side is a size() call
	if call, ok := left.(*filter.CallNode); ok && call.Op == filter.OpSize {
		return t.translateSizeComparison(op, call, right)
	}

	field, err := t.getFieldName(left)
	if err != nil {
		return "", err
	}

	if err = t.config.CheckComparison(left, right); err != nil {
		return "", err
	}

	rightResult, err := right.Accept(t)
	if err != nil {
		return "", err
	}
	rightStr, ok := rightResult.(string)
	if !ok {
		return "", coreerrs.Wrapf(filter.ErrInvalidExpression, "expected literal value, got %T", rightResult)
	}

	luaOp, err := t.luaOperator(op)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("(%s %s %s)", t.fieldRef(field), luaOp, rightStr), nil
}

// translateLogical handles && and || operators.
func (t *Translator) translateLogical(luaOp string, left, right filter.Node) (string, error) {
	l, err := t.getExprString(left)
	if err != nil {
		return "", err
	}
	r, err := t.getExprString(right)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("(%s %s %s)", l, luaOp, r), nil
}

// translateNot handles the ! operator.
func (t *Translator) translateNot(operand filter.Node) (string, error) {
	// For simple identifiers (like !active), treat as field ~= true
	if ident, ok := operand.(*filter.IdentNode); ok {
		field := ident.Name
		if !t.config.IsFieldAllowed(field) {
			return "", coreerrs.Wrapf(filter.ErrFieldNotAllowed, "%s", field)
		}
		mapped := t.config.ApplyFieldMapping(field)
		return fmt.Sprintf("(%s ~= %s)", t.fieldRef(mapped), luaTrue), nil
	}

	inner, err := t.getExprString(operand)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("(not %s)", inner), nil
}

// translateIn handles the in operator by expanding to an OR chain.
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

	ref := t.fieldRef(field)
	parts := make([]string, 0, len(valuesSlice))
	for _, v := range valuesSlice {
		s, ok := v.(string)
		if !ok {
			return "", coreerrs.Wrapf(filter.ErrInvalidExpression, "expected literal value in list, got %T", v)
		}
		parts = append(parts, fmt.Sprintf("%s == %s", ref, s))
	}

	return "(" + strings.Join(parts, " or ") + ")", nil
}

// translateContains handles field.contains("sub").
func (t *Translator) translateContains(target filter.Node, args []filter.Node) (string, error) {
	field, err := t.getFieldName(target)
	if err != nil {
		return "", err
	}
	s, err := t.getStringArg(args)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`(string.find(%s, %s, 1, true) ~= nil)`, t.fieldRef(field), `"`+escapeLuaString(s)+`"`), nil
}

// translateStartsWith handles field.startsWith("pre").
func (t *Translator) translateStartsWith(target filter.Node, args []filter.Node) (string, error) {
	return t.translateAffix(target, args, `(string.sub(%s, 1, %d) == %s)`)
}

// translateEndsWith handles field.endsWith("suf").
func (t *Translator) translateEndsWith(target filter.Node, args []filter.Node) (string, error) {
	return t.translateAffix(target, args, `(string.sub(%s, -%d) == %s)`)
}

// translateAffix renders a prefix or suffix comparison. Both slice the
// field to the needle's length and compare; only the string.sub bounds
// differ, which format carries as a template over (field reference,
// needle length, quoted needle).
func (t *Translator) translateAffix(target filter.Node, args []filter.Node, format string) (string, error) {
	field, err := t.getFieldName(target)
	if err != nil {
		return "", err
	}
	s, err := t.getStringArg(args)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(format, t.fieldRef(field), len(s), `"`+escapeLuaString(s)+`"`), nil
}

// sizeMarker is a type used to pass field names through the visitor for size() calls.
type sizeMarker string

// translateSize handles the size() function.
// Returns a sizeMarker for the parent binary op to handle.
func (t *Translator) translateSize(target filter.Node) (any, error) {
	field, err := t.getFieldName(target)
	if err != nil {
		return nil, err
	}
	return sizeMarker(field), nil
}

// translateSizeComparison handles size() comparisons like tags.size() == 3.
func (t *Translator) translateSizeComparison(op filter.Operator, call *filter.CallNode, right filter.Node) (string, error) {
	field, err := t.getFieldName(call.Target)
	if err != nil {
		return "", err
	}

	rightResult, err := right.Accept(t)
	if err != nil {
		return "", err
	}
	rightStr, ok := rightResult.(string)
	if !ok {
		return "", coreerrs.Wrapf(filter.ErrInvalidExpression, "expected literal value, got %T", rightResult)
	}

	luaOp, err := t.luaOperator(op)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("(#%s %s %s)", t.fieldRef(field), luaOp, rightStr), nil
}

// translateHas handles has(field) → field ~= nil.
func (t *Translator) translateHas(target filter.Node) (string, error) {
	field, err := t.getFieldName(target)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("(%s ~= nil)", t.fieldRef(field)), nil
}

// getExprString accepts a node and ensures the result is an expression string.
func (t *Translator) getExprString(node filter.Node) (string, error) {
	result, err := node.Accept(t)
	if err != nil {
		return "", err
	}
	s, ok := result.(string)
	if !ok {
		return "", coreerrs.Wrapf(filter.ErrInvalidExpression, "expected expression string, got %T", result)
	}
	return s, nil
}

// Ensure Translator implements filter.Visitor
var _ filter.Visitor = (*Translator)(nil)
