// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter

import (
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// CEL function names exposed by [SoftDeleteFilters].
const (
	SoftDeleteFuncNotDeleted  = "notDeleted"
	SoftDeleteFuncOnlyDeleted = "onlyDeleted"
)

// SoftDeleteFilters returns predicates for the canonical soft-delete
// pattern that pivots on the [TimestampFieldDeletedAt] field:
//
//	notDeleted()  → deletedAt == null
//	onlyDeleted() → deletedAt != null
//
// Equality with null matches both the "field is null" and "field is
// missing" cases under MongoDB and the in-memory evaluator, which is
// the semantics callers usually want for soft delete. If your storage
// distinguishes the two, gate explicitly with `has(deletedAt)` instead.
//
// The functions are not registered automatically — opt in explicitly
// via [RegisterFunctions] or [WithCustomFunctions]:
//
//	filter.RegisterFunctions(filter.SoftDeleteFilters())
//
// Each call returns a fresh map; callers can drop entries before
// passing it on.
func SoftDeleteFilters() map[string]CustomFunction {
	return map[string]CustomFunction{
		SoftDeleteFuncNotDeleted:  softDeleteNullCheck(OpEqual, SoftDeleteFuncNotDeleted),
		SoftDeleteFuncOnlyDeleted: softDeleteNullCheck(OpNotEqual, SoftDeleteFuncOnlyDeleted),
	}
}

// softDeleteNullCheck builds a no-arg handler that compares
// deletedAt against nil with the given operator.
func softDeleteNullCheck(op Operator, name string) CustomFunction {
	return func(args []Node) (Node, error) {
		if len(args) != 0 {
			return nil, coreerrs.Wrapf(ErrInvalidExpression,
				"%s expects 0 arguments, got %d", name, len(args))
		}
		return &BinaryOpNode{
			Op:    op,
			Left:  &IdentNode{Name: TimestampFieldDeletedAt},
			Right: &LiteralNode{Value: nil},
		}, nil
	}
}
