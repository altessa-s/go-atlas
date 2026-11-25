// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler

import (
	"context"
	"fmt"
	"math"

	"github.com/altessa-s/go-aip/filter"
	"github.com/altessa-s/go-horai/query"

	coremaps "github.com/altessa-s/go-atlas-core/collections/maps"
)

// Filterable fields of the task and history lists, as named in CEL filters.
var (
	taskFields    = query.TaskFields()
	historyFields = query.HistoryFields()
)

// comparisons maps the CEL comparison operators to query operators.
var comparisons = coremaps.NewImmutableMap(map[filter.Operator]query.Op{
	filter.OpEqual:    query.Eq,
	filter.OpNotEqual: query.Ne,
	filter.OpLT:       query.Lt,
	filter.OpLTE:      query.Le,
	filter.OpGT:       query.Gt,
	filter.OpGTE:      query.Ge,
})

// flipped mirrors a comparison whose literal is on the left ("5 < failures").
var flipped = coremaps.NewImmutableMap(map[query.Op]query.Op{
	query.Eq: query.Eq, query.Ne: query.Ne,
	query.Lt: query.Gt, query.Le: query.Ge, query.Gt: query.Lt, query.Ge: query.Le,
})

// FilterCompiler translates a nonempty filter into a scheduler query.
type FilterCompiler interface {
	Compile(context.Context, string) (query.Expr, error)
}

type celCompiler struct{ parser FilterParser }

func (c celCompiler) Compile(ctx context.Context, expr string) (query.Expr, error) {
	node, err := c.parser.Parse(ctx, expr)
	if err != nil {
		return nil, err
	}
	return toQuery(node)
}

// parseFilter compiles a filter expression and checks it against fields. An
// empty expression matches everything.
func (h *Handler) parseFilter(ctx context.Context, expr string, fields []query.Field) (query.Expr, error) {
	if expr == "" {
		return nil, nil //nolint:nilnil // a nil expression matches everything
	}
	q, err := h.compiler.Compile(ctx, expr)
	if err != nil {
		return nil, err
	}
	if err := query.Validate(q, fields); err != nil {
		return nil, err
	}
	return q, nil
}

// toQuery translates a parsed CEL expression into a query: &&, ||, !,
// comparisons and in between a field and literals, startsWith and contains
// on a string field, and a bare boolean field. Anything else fails with
// [filter.ErrUnsupportedOperation].
func toQuery(n filter.Node) (query.Expr, error) {
	switch v := n.(type) {
	case *filter.BinaryOpNode:
		return binaryToQuery(v)
	case *filter.UnaryOpNode:
		if v.Op != filter.OpNot {
			return nil, unsupported(v.Op)
		}
		inner, err := toQuery(v.Operand)
		if err != nil {
			return nil, err
		}
		return query.Not{Expr: inner}, nil
	case *filter.CallNode:
		return callToQuery(v)
	case *filter.IdentNode:
		return query.Cmp{Field: query.Field(v.Name), Op: query.Eq, Value: true}, nil
	default:
		return nil, fmt.Errorf("%w: %T", filter.ErrUnsupportedOperation, n)
	}
}

// binaryToQuery translates a logical, comparison or membership operation.
func binaryToQuery(n *filter.BinaryOpNode) (query.Expr, error) {
	switch n.Op {
	case filter.OpAnd, filter.OpOr:
		left, err := toQuery(n.Left)
		if err != nil {
			return nil, err
		}
		right, err := toQuery(n.Right)
		if err != nil {
			return nil, err
		}
		if n.Op == filter.OpAnd {
			return query.And{left, right}, nil
		}
		return query.Or{left, right}, nil
	case filter.OpIn:
		field, ok := n.Left.(*filter.IdentNode)
		list, isList := n.Right.(*filter.ListNode)
		if !ok || !isList {
			return nil, unsupported(n.Op)
		}
		values, err := listValue(list)
		if err != nil {
			return nil, err
		}
		return query.Cmp{Field: query.Field(field.Name), Op: query.In, Value: values}, nil
	default:
		// Comparisons are translated below.
	}
	op, ok := comparisons.Get(n.Op)
	if !ok {
		return nil, unsupported(n.Op)
	}
	field, lit := n.Left, n.Right
	if _, isField := field.(*filter.IdentNode); !isField {
		field, lit = n.Right, n.Left
		op, _ = flipped.Get(op)
	}
	ident, ok := field.(*filter.IdentNode)
	literal, isLit := lit.(*filter.LiteralNode)
	if !ok || !isLit {
		return nil, unsupported(n.Op)
	}
	value, err := scalar(literal.Value)
	if err != nil {
		return nil, err
	}
	return query.Cmp{Field: query.Field(ident.Name), Op: op, Value: value}, nil
}

// callToQuery translates field.startsWith("x") and field.contains("x").
func callToQuery(n *filter.CallNode) (query.Expr, error) {
	var op query.Op
	switch n.Op {
	case filter.OpStartsWith:
		op = query.Prefix
	case filter.OpContains:
		op = query.Contains
	default:
		return nil, unsupported(n.Op)
	}
	ident, ok := n.Target.(*filter.IdentNode)
	if !ok || len(n.Args) != 1 {
		return nil, unsupported(n.Op)
	}
	lit, ok := n.Args[0].(*filter.LiteralNode)
	if !ok {
		return nil, unsupported(n.Op)
	}
	s, ok := lit.Value.(string)
	if !ok {
		return nil, fmt.Errorf("%w: %s needs a string", filter.ErrFieldTypeMismatch, n.Op)
	}
	return query.Cmp{Field: query.Field(ident.Name), Op: op, Value: s}, nil
}

// scalar normalizes a literal to the string, int64 or bool a query holds.
func scalar(v any) (any, error) {
	switch x := v.(type) {
	case string, bool, int64:
		return x, nil
	case int:
		return int64(x), nil
	case uint64:
		if x > math.MaxInt64 {
			return nil, fmt.Errorf("%w: %d overflows int64", filter.ErrUnsupportedType, x)
		}
		return int64(x), nil
	case float64:
		if x != math.Trunc(x) || x < math.MinInt64 || x >= 0x1p63 {
			return nil, fmt.Errorf("%w: %v is not an integer", filter.ErrUnsupportedType, x)
		}
		return int64(x), nil
	default:
		return nil, fmt.Errorf("%w: %T", filter.ErrUnsupportedType, v)
	}
}

// listValue converts a list of literals of one kind to the typed slice an In
// comparison holds.
func listValue(list *filter.ListNode) (any, error) {
	var (
		strs  []string
		ints  []int64
		bools []bool
	)
	for _, e := range list.Elements {
		lit, ok := e.(*filter.LiteralNode)
		if !ok {
			return nil, unsupported(filter.OpIn)
		}
		v, err := scalar(lit.Value)
		if err != nil {
			return nil, err
		}
		switch x := v.(type) {
		case string:
			strs = append(strs, x)
		case int64:
			ints = append(ints, x)
		case bool:
			bools = append(bools, x)
		}
	}
	switch n := len(list.Elements); {
	case n > 0 && len(strs) == n:
		return strs, nil
	case n > 0 && len(ints) == n:
		return ints, nil
	case n > 0 && len(bools) == n:
		return bools, nil
	default:
		return nil, fmt.Errorf("%w: in needs a non-empty list of one type", filter.ErrUnsupportedType)
	}
}

// unsupported reports an operator the scheduler queries cannot express.
func unsupported(op filter.Operator) error {
	return fmt.Errorf("%w: %s", filter.ErrUnsupportedOperation, op)
}
