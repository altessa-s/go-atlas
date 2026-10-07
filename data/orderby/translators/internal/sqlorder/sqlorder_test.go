// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqlorder_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/orderby"
	"github.com/altessa-s/go-atlas/data/orderby/translators/internal/sqlorder"
	"github.com/altessa-s/go-atlas/internal/sqldialect"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func context(tb testing.TB, opts ...orderby.TranslatorOption) *orderby.TranslatorContext {
	tb.Helper()
	cfg, err := orderby.NewTranslatorContext(opts...)
	require.NoError(tb, err)
	return cfg
}

func TestTranslate(t *testing.T) {
	t.Parallel()
	cfg := context(t, orderby.WithFieldMapping(map[string]string{"bad": "a b"}))
	for _, tc := range []struct {
		name  string
		style sqldialect.Style
		in    string
		want  string
		err   error
	}{
		{name: "postgres", style: sqldialect.Postgres, in: "a desc, t.b", want: `"a" DESC, "t"."b" ASC`},
		{name: "mysql", style: sqldialect.MySQL, in: "a desc, t.b", want: "`a` DESC, `t`.`b` ASC"},
		{name: "invalid mapping", style: sqldialect.Postgres, in: "bad", err: orderby.ErrInvalidFieldPath},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := sqlorder.Translate(cfg, tc.style, testhelpers.MustParseOrderBy(t, tc.in))
			require.ErrorIs(t, err, tc.err)
			require.Equal(t, tc.want, got)
		})
	}

	got, err := sqlorder.Translate(cfg, sqldialect.Postgres, orderby.Spec{})
	require.NoError(t, err)
	require.Empty(t, got)

	strict := context(t, orderby.WithAllowedFields("a"), orderby.WithUntrustedInput())
	_, err = sqlorder.Translate(strict, sqldialect.Postgres, testhelpers.MustParseOrderBy(t, "a, b"))
	require.ErrorIs(t, err, orderby.ErrFieldNotAllowed)
}
