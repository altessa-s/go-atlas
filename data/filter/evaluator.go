// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter

import (
	"fmt"
	"regexp"
	"strings"
)

// Evaluator evaluates a filter AST against an in-memory map.
type Evaluator struct {
	config *TranslatorConfig
	data   map[string]any
	depth  int
}

// NewEvaluator creates a new in-memory evaluator with the given options.
func NewEvaluator(opts ...TranslatorOption) *Evaluator {
	cfg := NewTranslatorConfig()
	for _, opt := range opts {
		opt(cfg)
	}
	return &Evaluator{config: cfg}
}

// Evaluate returns true if data matches the filter node.
func (e *Evaluator) Evaluate(node Node, data map[string]any) (bool, error) {
	e.data = data
	e.depth = 0
	result, err := node.Accept(e)
	if err != nil {
		return false, err
	}
	b, ok := result.(bool)
	if !ok {
		return false, fmt.Errorf("%w: expression must evaluate to bool, got %T", ErrInvalidExpression, result)
	}
	return b, nil
}

// VisitLiteral returns the literal value.
func (e *Evaluator) VisitLiteral(n *LiteralNode) (any, error) {
	return n.Value, nil
}

// VisitIdent looks up the field in the data map, supporting dot notation.
func (e *Evaluator) VisitIdent(n *IdentNode) (any, error) {
	field := n.Name
	if !e.config.IsFieldAllowed(field) {
		return nil, fmt.Errorf("%w: %s", ErrFieldNotAllowed, field)
	}
	mapped := e.config.ApplyFieldMapping(field)
	return lookupField(e.data, mapped), nil
}

// VisitBinaryOp evaluates binary operations.
func (e *Evaluator) VisitBinaryOp(n *BinaryOpNode) (any, error) {
	if err := e.checkDepth(); err != nil {
		return nil, err
	}
	e.depth++
	defer func() { e.depth-- }()

	switch n.Op {
	case OpAnd:
		return e.evalLogicalAnd(n.Left, n.Right)
	case OpOr:
		return e.evalLogicalOr(n.Left, n.Right)
	case OpIn:
		return e.evalIn(n.Left, n.Right)
	default:
		return e.evalComparison(n.Op, n.Left, n.Right)
	}
}

// VisitUnaryOp evaluates unary operations.
func (e *Evaluator) VisitUnaryOp(n *UnaryOpNode) (any, error) {
	if err := e.checkDepth(); err != nil {
		return nil, err
	}
	e.depth++
	defer func() { e.depth-- }()

	if n.Op != OpNot {
		return nil, fmt.Errorf("%w: unary operator %v", ErrUnsupportedOperation, n.Op)
	}

	val, err := n.Operand.Accept(e)
	if err != nil {
		return nil, err
	}
	b, ok := val.(bool)
	if !ok {
		return nil, fmt.Errorf("%w: ! requires bool operand, got %T", ErrInvalidExpression, val)
	}
	return !b, nil
}

// VisitCall evaluates function calls.
func (e *Evaluator) VisitCall(n *CallNode) (any, error) {
	if err := e.checkDepth(); err != nil {
		return nil, err
	}
	e.depth++
	defer func() { e.depth-- }()

	switch n.Op {
	case OpContains:
		return e.evalStringFunc(n, strings.Contains)
	case OpStartsWith:
		return e.evalStringFunc(n, strings.HasPrefix)
	case OpEndsWith:
		return e.evalStringFunc(n, strings.HasSuffix)
	case OpMatches:
		return e.evalMatches(n)
	case OpHas, OpExists:
		return e.evalHas(n)
	case OpSize:
		return e.evalSize(n)
	default:
		return nil, fmt.Errorf("%w: function %v", ErrUnsupportedOperation, n.Op)
	}
}

// VisitList evaluates a list literal.
func (e *Evaluator) VisitList(n *ListNode) (any, error) {
	result := make([]any, 0, len(n.Elements))
	for _, elem := range n.Elements {
		val, err := elem.Accept(e)
		if err != nil {
			return nil, err
		}
		result = append(result, val)
	}
	return result, nil
}

// evalLogicalAnd short-circuits on false.
func (e *Evaluator) evalLogicalAnd(left, right Node) (any, error) {
	lv, err := left.Accept(e)
	if err != nil {
		return nil, err
	}
	lb, ok := lv.(bool)
	if !ok {
		return nil, fmt.Errorf("%w: && requires bool operands", ErrInvalidExpression)
	}
	if !lb {
		return false, nil
	}
	rv, err := right.Accept(e)
	if err != nil {
		return nil, err
	}
	rb, ok := rv.(bool)
	if !ok {
		return nil, fmt.Errorf("%w: && requires bool operands", ErrInvalidExpression)
	}
	return rb, nil
}

// evalLogicalOr short-circuits on true.
func (e *Evaluator) evalLogicalOr(left, right Node) (any, error) {
	lv, err := left.Accept(e)
	if err != nil {
		return nil, err
	}
	lb, ok := lv.(bool)
	if !ok {
		return nil, fmt.Errorf("%w: || requires bool operands", ErrInvalidExpression)
	}
	if lb {
		return true, nil
	}
	rv, err := right.Accept(e)
	if err != nil {
		return nil, err
	}
	rb, ok := rv.(bool)
	if !ok {
		return nil, fmt.Errorf("%w: || requires bool operands", ErrInvalidExpression)
	}
	return rb, nil
}

// evalComparison evaluates comparison operators.
func (e *Evaluator) evalComparison(op Operator, left, right Node) (any, error) {
	lv, err := left.Accept(e)
	if err != nil {
		return nil, err
	}
	rv, err := right.Accept(e)
	if err != nil {
		return nil, err
	}
	return compare(op, lv, rv)
}

// evalIn checks if the left value is in the right list.
func (e *Evaluator) evalIn(left, right Node) (any, error) {
	lv, err := left.Accept(e)
	if err != nil {
		return nil, err
	}
	rv, err := right.Accept(e)
	if err != nil {
		return nil, err
	}
	list, ok := rv.([]any)
	if !ok {
		return nil, fmt.Errorf("%w: in requires a list on the right side", ErrInvalidExpression)
	}
	for _, item := range list {
		if valuesEqual(lv, item) {
			return true, nil
		}
	}
	return false, nil
}

// evalStringFunc evaluates contains/startsWith/endsWith.
func (e *Evaluator) evalStringFunc(n *CallNode, fn func(string, string) bool) (any, error) {
	target, err := n.Target.Accept(e)
	if err != nil {
		return nil, err
	}
	s, ok := target.(string)
	if !ok {
		return false, nil
	}
	if len(n.Args) != 1 {
		return nil, fmt.Errorf("%w: string function requires exactly 1 argument", ErrInvalidExpression)
	}
	arg, err := n.Args[0].Accept(e)
	if err != nil {
		return nil, err
	}
	substr, ok := arg.(string)
	if !ok {
		return nil, fmt.Errorf("%w: string function argument must be a string", ErrInvalidExpression)
	}
	return fn(s, substr), nil
}

// evalMatches evaluates regex matching.
func (e *Evaluator) evalMatches(n *CallNode) (any, error) {
	target, err := n.Target.Accept(e)
	if err != nil {
		return nil, err
	}
	s, ok := target.(string)
	if !ok {
		return false, nil
	}
	if len(n.Args) != 1 {
		return nil, fmt.Errorf("%w: matches() requires exactly 1 argument", ErrInvalidExpression)
	}
	arg, err := n.Args[0].Accept(e)
	if err != nil {
		return nil, err
	}
	pattern, ok := arg.(string)
	if !ok {
		return nil, fmt.Errorf("%w: matches() argument must be a string", ErrInvalidExpression)
	}
	matched, err := regexp.MatchString(pattern, s)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid regex: %v", ErrInvalidExpression, err)
	}
	return matched, nil
}

// evalHas checks if a field exists (non-nil) in the data.
func (e *Evaluator) evalHas(n *CallNode) (any, error) {
	ident, ok := n.Target.(*IdentNode)
	if !ok {
		return nil, fmt.Errorf("%w: has() requires an identifier", ErrInvalidExpression)
	}
	field := ident.Name
	if !e.config.IsFieldAllowed(field) {
		return nil, fmt.Errorf("%w: %s", ErrFieldNotAllowed, field)
	}
	mapped := e.config.ApplyFieldMapping(field)
	return fieldExists(e.data, mapped), nil
}

// evalSize returns the size of a string or slice.
func (e *Evaluator) evalSize(n *CallNode) (any, error) {
	target, err := n.Target.Accept(e)
	if err != nil {
		return nil, err
	}
	switch v := target.(type) {
	case string:
		return int64(len(v)), nil
	case []any:
		return int64(len(v)), nil
	default:
		return nil, fmt.Errorf("%w: size() not supported for %T", ErrUnsupportedOperation, target)
	}
}

// lookupField resolves a potentially dotted field path in the data map.
func lookupField(data map[string]any, field string) any {
	parts := strings.Split(field, ".")
	var current any = data
	for _, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current, ok = m[part]
		if !ok {
			return nil
		}
	}
	return current
}

// fieldExists checks whether a dotted field path exists in the data map.
func fieldExists(data map[string]any, field string) bool {
	parts := strings.Split(field, ".")
	var current any = data
	for _, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return false
		}
		current, ok = m[part]
		if !ok {
			return false
		}
	}
	return true
}

// compare performs typed comparison.
func compare(op Operator, left, right any) (bool, error) {
	// Handle nil
	if left == nil || right == nil {
		return compareNil(op, left, right)
	}

	// Normalize numeric types for comparison
	ln, lok := toFloat64(left)
	rn, rok := toFloat64(right)
	if lok && rok {
		return compareOrdered(op, ln, rn)
	}

	// String comparison
	ls, lok := left.(string)
	rs, rok := right.(string)
	if lok && rok {
		return compareOrdered(op, ls, rs)
	}

	// Bool comparison (== and != only)
	lb, lok := left.(bool)
	rb, rok := right.(bool)
	if lok && rok {
		switch op {
		case OpEqual:
			return lb == rb, nil
		case OpNotEqual:
			return lb != rb, nil
		default:
			return false, fmt.Errorf("%w: cannot use %v on booleans", ErrUnsupportedOperation, op)
		}
	}

	return false, fmt.Errorf("%w: cannot compare %T and %T", ErrUnsupportedType, left, right)
}

func compareNil(op Operator, left, right any) (bool, error) {
	switch op {
	case OpEqual:
		return left == nil && right == nil, nil
	case OpNotEqual:
		return !(left == nil && right == nil), nil
	default:
		return false, fmt.Errorf("%w: cannot use %v with nil", ErrUnsupportedOperation, op)
	}
}

type ordered interface {
	~int64 | ~float64 | ~string
}

func compareOrdered[T ordered](op Operator, a, b T) (bool, error) {
	switch op {
	case OpEqual:
		return a == b, nil
	case OpNotEqual:
		return a != b, nil
	case OpLT:
		return a < b, nil
	case OpLTE:
		return a <= b, nil
	case OpGT:
		return a > b, nil
	case OpGTE:
		return a >= b, nil
	default:
		return false, fmt.Errorf("%w: operator %v", ErrUnsupportedOperation, op)
	}
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case int64:
		return float64(n), true
	case uint64:
		return float64(n), true
	case float64:
		return n, true
	case int32:
		return float64(n), true
	case int:
		return float64(n), true
	default:
		return 0, false
	}
}

func valuesEqual(a, b any) bool {
	an, aok := toFloat64(a)
	bn, bok := toFloat64(b)
	if aok && bok {
		return an == bn
	}
	return a == b
}

func (e *Evaluator) checkDepth() error {
	if e.depth >= e.config.MaxDepth() {
		return fmt.Errorf("%w: depth %d exceeds maximum %d", ErrMaxDepthExceeded, e.depth, e.config.MaxDepth())
	}
	return nil
}

// Ensure Evaluator implements Visitor.
var _ Visitor = (*Evaluator)(nil)
