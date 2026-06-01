// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lua

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// TestTranslator_WithFieldTypes is the cross-backend regression guard
// PR-45 review requested. The type-check hook in translator.go is
// byte-identical across all four backends, so a future delete of the
// CheckComparison call in one backend would silently slip past CI
// without a per-backend smoke test. This file mirrors the existing
// mongo translator_test.go fixture: a small schema, a few match
// expressions, a few mismatch expressions, asserts
// ErrFieldTypeMismatch via errors.Is.
func TestTranslator_WithFieldTypes(t *testing.T) {
	trans := mustTranslator(t, "d", filter.WithFieldTypes(map[string]filter.FieldKind{
		"status":    filter.FieldKindInt,
		"active":    filter.FieldKindBool,
		"name":      filter.FieldKindString,
		"price":     filter.FieldKindFloat,
		"createdAt": filter.FieldKindTimestamp,
	}))

	t.Run("match", func(t *testing.T) {
		for _, expr := range []string{
			`status == 1`,
			`active == true`,
			`name == "Alice"`,
			`price >= 100`,
			`status in [1, 2, 3]`,
			`other == "anything"`, // field not in schema → skipped
		} {
			t.Run(expr, func(t *testing.T) {
				node := testhelpers.MustParseFilter(t, expr)
				_, err := trans.Translate(node)
				require.NoError(t, err)
			})
		}
	})

	t.Run("mismatch", func(t *testing.T) {
		for _, expr := range []string{
			`status == "qwer"`,
			`active == 1`,
			`name == 42`,
			`status in [1, "qwer", 3]`,
			`"qwer" == status`, // mirrored ordering must also fail
		} {
			t.Run(expr, func(t *testing.T) {
				node := testhelpers.MustParseFilter(t, expr)
				_, err := trans.Translate(node)
				require.ErrorIs(t, err, filter.ErrFieldTypeMismatch)
			})
		}
	})
}
