// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter

import (
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// BetweenFunc is the CEL function name registered by [BetweenFilter].
const BetweenFunc = "between"

// Between returns a [CustomFunction] that expands a CEL call
//
//	between(field, lo, hi)
//
// into the canonical AST shape `field >= lo && field <= hi`. The first
// argument must be a field identifier; the remaining two are passed
// through as the bounds and may be any expression the surrounding
// translator accepts (literals, timestamps, dotted field paths).
//
// Backend translators see a regular And-tree of comparisons, so the
// helper works uniformly across mongo, redisearch, and lua.
func Between() CustomFunction {
	return func(args []Node) (Node, error) {
		const wantArgs = 3
		if len(args) != wantArgs {
			return nil, coreerrs.Wrapf(ErrInvalidExpression,
				"between expects %d arguments (field, lo, hi), got %d", wantArgs, len(args))
		}
		field, ok := args[0].(*IdentNode)
		if !ok {
			return nil, coreerrs.Wrapf(ErrInvalidExpression,
				"between: first argument must be a field identifier, got %T", args[0])
		}
		return &BinaryOpNode{
			Op:    OpAnd,
			Left:  &BinaryOpNode{Op: OpGTE, Left: field, Right: args[1]},
			Right: &BinaryOpNode{Op: OpLTE, Left: field, Right: args[2]},
		}, nil
	}
}

// BetweenFilter returns the [Between] handler as a one-entry map
// suitable for [RegisterFunctions] or [WithCustomFunctions]:
//
//	filter.RegisterFunctions(filter.BetweenFilter())
//
// Each call returns a fresh map.
func BetweenFilter() map[string]CustomFunction {
	return map[string]CustomFunction{BetweenFunc: Between()}
}
