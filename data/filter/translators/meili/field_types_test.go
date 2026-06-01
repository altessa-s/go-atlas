// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package meili

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// TestTranslator_WithFieldTypes is the cross-backend regression guard
// PR-45 review requested. See data/filter/translators/lua for the
// design rationale — the byte-identical CheckComparison call needs
// per-backend coverage so a future regression cannot delete it
// silently.
func TestTranslator_WithFieldTypes(t *testing.T) {
	trans := mustTranslator(t, filter.WithFieldTypes(map[string]filter.FieldKind{
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
			`other == "anything"`,
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
			`"qwer" == status`,
		} {
			t.Run(expr, func(t *testing.T) {
				node := testhelpers.MustParseFilter(t, expr)
				_, err := trans.Translate(node)
				require.ErrorIs(t, err, filter.ErrFieldTypeMismatch)
			})
		}
	})
}
