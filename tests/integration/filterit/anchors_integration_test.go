// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filterit_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/tests/integration/filterit"
)

// rowInserter is a backend that can store rows beyond the shared dataset.
type rowInserter interface {
	backend
	insert(tb testing.TB, rows []filterit.Row)
}

// anchorRows put a newline at the end of one name and in the middle of the
// other: PCRE and ICU let `$` match before a final newline, RE2 — and so
// CEL — does not.
func anchorRows() []filterit.Row {
	rows := []filterit.Row{
		{ID: 101, Name: "Ann\n"},
		{ID: 102, Name: "x\nAnn"},
	}
	for i := range rows {
		rows[i].Status, rows[i].CreatedAt = "active", filterit.Epoch
	}
	return rows
}

// TestRegexAnchors checks that string predicates and matches() anchor at the
// very end of the text, as CEL does, on every backend that has them. Each
// expectation is first checked against the in-memory evaluator, so the
// table is CEL truth.
func TestRegexAnchors(t *testing.T) {
	t.Parallel()

	cases := map[string][]int64{
		`name.endsWith("Ann")`:      {102},
		`name.endsWith("Ann\n")`:    {101},
		`name.startsWith("Ann")`:    {101},
		`name.matches("Ann$")`:      {102},
		`name.matches("^Ann$")`:     nil,
		`name.matches("(?m)^Ann$")`: {101, 102},
		`name.matches("^x")`:        {102},
	}

	evaluator, err := filter.NewEvaluator()
	require.NoError(t, err)
	for expr, want := range cases {
		var got []int64
		for _, row := range anchorRows() {
			ok, err := evaluator.Evaluate(parse(t, expr), map[string]any{"name": row.Name})
			require.NoError(t, err, expr)
			if ok {
				got = append(got, row.ID)
			}
		}
		require.Equal(t, want, got, "CEL truth for %q", expr)
	}

	for _, b := range []rowInserter{&mongoBackend{}, &mariadbBackend{}, &postgresBackend{}, &clickhouseBackend{}} {
		t.Run(string(b.name()), func(t *testing.T) {
			t.Parallel()
			b.setup(t)
			b.insert(t, anchorRows())
			for expr, want := range cases {
				got, err := b.search(t, `id > 100 && `+expr)
				require.NoError(t, err, expr)
				require.Equal(t, want, normalize(slices.Clone(got)), expr)
			}
		})
	}
}
