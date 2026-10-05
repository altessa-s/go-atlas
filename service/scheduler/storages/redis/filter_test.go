// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/service/scheduler"
)

// TestFilterPlanPushdown pins which part of a task filter reaches RediSearch:
// only comparisons of always-present numeric fields, never a predicate whose
// RediSearch form disagrees with CEL.
func TestFilterPlanPushdown(t *testing.T) {
	t.Parallel()
	for expr, want := range map[string]string{
		`status == 1`:              `@status:[1 1]`,
		`status == 1 && id == "a"`: `@status:[1 1]`,
		`id == "a" && (priority > 0 && failures <= 2)`: `(@priority:[(0 +inf] @failures:[-inf 2])`,
		`!(status > 1) || priority in [1, 2]`:          `((-(@status:[(1 +inf]))|((@priority:[1 1]|@priority:[2 2])))`,
		`status == 1 || id == "a"`:                     ``,
		`status in []`:                                 ``,
		`!(status in [])`:                              ``,
		`nextRunAt == 0`:                               ``,
		`oneShot`:                                      ``,
		`id.startsWith("ab")`:                          ``,
		// Data-dependent: evaluated only against fetched documents, never
		// against a synthetic empty one, where it would be out of range.
		`description.substring(0, 1) == "A"`: ``,
		`status == priority`:                 ``,
	} {
		t.Run(expr, func(t *testing.T) {
			t.Parallel()
			plan, err := newFilterPlan(testhelpers.MustParseFilter(t, expr), scheduler.TaskFilterFields, taskFieldSchema,
				taskExactFields)
			require.NoError(t, err)
			require.Equal(t, want, plan.query)
		})
	}
}

// TestFilterPlanRejectsUnknownField checks the up-front field check, including
// a reference the evaluator would short-circuit past.
func TestFilterPlanRejectsUnknownField(t *testing.T) {
	t.Parallel()
	for _, expr := range []string{`secret == 1`, `status == 1 || secret == 1`, `id.startsWith(secret)`} {
		_, err := newFilterPlan(testhelpers.MustParseFilter(t, expr), scheduler.TaskFilterFields, taskFieldSchema, taskExactFields)
		require.ErrorIs(t, err, filter.ErrFieldNotAllowed, expr)
	}
}
