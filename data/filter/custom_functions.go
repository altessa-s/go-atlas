// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter

import (
	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// CustomFunction expands a CEL call of the form name(args...) into an
// arbitrary AST node. The handler receives already converted argument
// nodes and is responsible for validating arity and types. The node it
// returns replaces the original call in the AST before the filter
// reaches any [Evaluator] or translator.
//
// See [WithCustomFunctions] for registration and [CompareField] for the
// canonical "field op arg" shortcut.
type CustomFunction func(args []Node) (Node, error)

// CompareField returns a [CustomFunction] that expands name(arg) into
// BinaryOpNode{op, IdentNode(field), arg}. It is the canonical building
// block for semantic shortcuts such as createdAfter or updatedBefore:
//
//	filter.WithCustomFunctions(map[string]filter.CustomFunction{
//	    "createdAfter":  filter.CompareField("createdAt", filter.OpGT),
//	    "updatedBefore": filter.CompareField("updatedAt", filter.OpLT),
//	})
//
// op must be a comparison operator (see [Operator.IsComparison]);
// otherwise the handler returns an error wrapping
// [ErrInvalidExpression] when invoked.
func CompareField(field string, op Operator) CustomFunction {
	return func(args []Node) (Node, error) {
		if len(args) != 1 {
			return nil, coreerrs.Wrapf(ErrInvalidExpression,
				"function expects 1 argument, got %d", len(args))
		}
		if !op.IsComparison() {
			return nil, coreerrs.Wrapf(ErrInvalidExpression,
				"operator %v is not a comparison", op)
		}
		return &BinaryOpNode{Op: op, Left: &IdentNode{Name: field}, Right: args[0]}, nil
	}
}

// reservedFunctionNames lists CEL built-ins that callers must not
// override via [WithCustomFunctions]. Kept in sync with the explicit
// switch branches in [Parser.convertCall].
var reservedFunctionNames = coremaps.NewImmutableMap(map[string]struct{}{
	"contains":   {},
	"startsWith": {},
	"endsWith":   {},
	"matches":    {},
	"size":       {},
	"has":        {},
	"timestamp":  {},
})

// validateCustomFunctions rejects nil handlers and names that collide
// with reserved CEL built-ins. It is called from [NewParser] so a
// misconfiguration surfaces at construction time rather than during
// parsing.
func validateCustomFunctions(funcs map[string]CustomFunction) error {
	for name, h := range funcs {
		if h == nil {
			return coreerrs.Wrapf(ErrInvalidExpression,
				"custom function %q: nil handler", name)
		}
		if reservedFunctionNames.Contains(name) {
			return coreerrs.Wrapf(ErrInvalidExpression,
				"custom function %q collides with built-in", name)
		}
	}
	return nil
}
