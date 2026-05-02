// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter

import (
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Constant returns a [CustomFunction] that takes no arguments and
// expands a CEL call name() into BinaryOpNode{op, IdentNode(field),
// LiteralNode(value)}. It is the building block for stable, parameter
// less predicates over a domain enum or a fixed threshold:
//
//	filter.RegisterFunctions(map[string]filter.CustomFunction{
//	    "isActive":     filter.Constant("status",   filter.OpEqual, "active"),
//	    "isPending":    filter.Constant("status",   filter.OpEqual, "pending"),
//	    "hasFailures":  filter.Constant("failures", filter.OpGT,    int64(0)),
//	})
//
// Pass [nil] as value to express explicit null comparisons (the same
// shape that powers [SoftDeleteFilters]). op must be a comparison
// operator (see [Operator.IsComparison]); otherwise the handler
// returns an error wrapping [ErrInvalidExpression] when invoked.
//
// Calling the resulting handler with any arguments is also rejected,
// since by construction the predicate is parameter-less.
func Constant(field string, op Operator, value any) CustomFunction {
	return func(args []Node) (Node, error) {
		if len(args) != 0 {
			return nil, coreerrs.Wrapf(ErrInvalidExpression,
				"constant predicate expects 0 arguments, got %d", len(args))
		}
		if !op.IsComparison() {
			return nil, coreerrs.Wrapf(ErrInvalidExpression,
				"operator %v is not a comparison", op)
		}
		return &BinaryOpNode{
			Op:    op,
			Left:  &IdentNode{Name: field},
			Right: &LiteralNode{Value: value},
		}, nil
	}
}
