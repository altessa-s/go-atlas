// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqlbase

import (
	"fmt"

	"github.com/altessa-s/go-atlas/data/filter"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// StringPredicates is a dialect's rendering of the four string
// predicates, as templates over the quoted column and the operand.
//
// The verbs are indexed because the operand does not always follow the
// column — MariaDB's LOCATE takes the needle first. %[1]s is the column
// and %[2]s the operand; an EndsWith template that repeats the operand
// takes each further rendering of it as %[3]s, %[4]s and so on.
type StringPredicates struct {
	Contains   string
	StartsWith string
	EndsWith   string
	Matches    string

	// EndsWithBinds is the number of times the EndsWith template
	// references the operand; zero means once. Neither MariaDB nor
	// PostgreSQL has a suffix predicate, so both compile it to a
	// comparison: PostgreSQL needs the needle twice — once to size the
	// suffix, once to compare against it — and MariaDB a third time for
	// the length guard that keeps PAD SPACE collations exact. ClickHouse
	// has endsWith() and needs it once.
	//
	// It is declared rather than inferred from the template on purpose. A
	// wrong bind count misaligns every argument after it, and under
	// PostgreSQL's numbered placeholders that corrupts the query silently
	// instead of failing.
	EndsWithBinds int
}

// RenderStringPredicate renders one string predicate from a dialect's
// template table.
//
// value is called exactly once per operand occurrence in the chosen
// template, which is what keeps the argument slice aligned with the
// emitted text.
func RenderStringPredicate(
	op filter.Operator, col, needle string, value ValueFunc, tpl StringPredicates,
) (string, error) {
	arg, err := value(needle)
	if err != nil {
		return "", err
	}

	switch op {
	case filter.OpContains:
		return fmt.Sprintf(tpl.Contains, col, arg), nil
	case filter.OpStartsWith:
		return fmt.Sprintf(tpl.StartsWith, col, arg), nil
	case filter.OpMatches:
		return fmt.Sprintf(tpl.Matches, col, arg), nil
	case filter.OpEndsWith:
		args := []any{col, arg}
		for range tpl.EndsWithBinds - 1 {
			next, err := value(needle)
			if err != nil {
				return "", err
			}
			args = append(args, next)
		}
		return fmt.Sprintf(tpl.EndsWith, args...), nil
	default:
		return "", coreerrs.Wrapf(filter.ErrUnsupportedOperation, "string predicate %v", op)
	}
}
