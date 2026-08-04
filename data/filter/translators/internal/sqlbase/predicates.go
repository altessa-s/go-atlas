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
// column — MariaDB's LOCATE takes the needle first. %[1]s is the column,
// %[2]s the operand, and %[3]s a second rendering of the same operand
// for a dialect that has to repeat it.
type StringPredicates struct {
	Contains   string
	StartsWith string
	EndsWith   string
	Matches    string

	// EndsWithBindsTwice marks a dialect whose EndsWith template
	// references the operand twice. Neither MariaDB nor PostgreSQL has a
	// suffix predicate, so both compile it to a comparison that needs the
	// needle once to size the suffix and once to compare against it;
	// ClickHouse has endsWith() and needs it once.
	//
	// It is declared rather than inferred from the template on purpose. A
	// wrong bind count misaligns every argument after it, and under
	// PostgreSQL's numbered placeholders that corrupts the query silently
	// instead of failing.
	EndsWithBindsTwice bool
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
		if !tpl.EndsWithBindsTwice {
			return fmt.Sprintf(tpl.EndsWith, col, arg), nil
		}
		second, err := value(needle)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf(tpl.EndsWith, col, arg, second), nil
	default:
		return "", coreerrs.Wrapf(filter.ErrUnsupportedOperation, "string predicate %v", op)
	}
}
