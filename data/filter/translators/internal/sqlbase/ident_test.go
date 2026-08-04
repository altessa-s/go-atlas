// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqlbase_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/data/filter/translators/internal/sqlbase"
)

func TestValidateIdent(t *testing.T) {
	t.Parallel()

	t.Run("accepts", func(t *testing.T) {
		t.Parallel()

		for _, name := range []string{
			"name",
			"_name",
			"n4me",
			"created_at",
			"address.city",
			"user.profile.name",
			"A.B",
		} {
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				require.NoError(t, sqlbase.ValidateIdent(name))
			})
		}
	})

	// Most of these would have been neutralized by quoting alone. They
	// are refused anyway: the column position is the one part of a
	// generated clause no placeholder can cover, so it accepts plain
	// identifiers and nothing else.
	t.Run("rejects", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			label string
			name  string
		}{
			{"empty", ""},
			{"bare dot", "."},
			{"empty segment", "address..city"},
			{"leading dot", ".city"},
			{"trailing dot", "address."},
			{"leading digit", "1name"},
			{"whitespace", "na me"},
			{"call expression", "lower(name)"},
			{"subscript", "metadata['x']"},
			{"backtick break-out", "name` = '' OR 1 = 1"},
			{"double-quote break-out", `name" = '' OR 1 = 1`},
			{"comment", "name--"},
			{"non-ASCII", "naïve"},
		}

		for _, tt := range tests {
			t.Run(tt.label, func(t *testing.T) {
				t.Parallel()
				require.ErrorIs(t, sqlbase.ValidateIdent(tt.name), filter.ErrInvalidExpression)
			})
		}
	})
}

func TestQuoteWhole(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		quote byte
		want  string
	}{
		{"simple", "name", '`', "`name`"},
		{"dots stay inside one identifier", "address.city", '`', "`address.city`"},
		{"double quote", "name", '"', `"name"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := sqlbase.QuoteWhole(tt.input, tt.quote)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}

	t.Run("propagates validation failure", func(t *testing.T) {
		t.Parallel()
		_, err := sqlbase.QuoteWhole("lower(name)", '`')
		require.ErrorIs(t, err, filter.ErrInvalidExpression)
	})
}

func TestQuoteQualified(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		quote byte
		want  string
	}{
		{"simple backtick", "name", '`', "`name`"},
		{"simple double quote", "name", '"', `"name"`},
		{"two segments", "address.city", '`', "`address`.`city`"},
		{"three segments", "user.profile.name", '"', `"user"."profile"."name"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := sqlbase.QuoteQualified(tt.input, tt.quote)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}

	t.Run("propagates validation failure", func(t *testing.T) {
		t.Parallel()
		_, err := sqlbase.QuoteQualified("address..city", '"')
		require.ErrorIs(t, err, filter.ErrInvalidExpression)
	})
}
