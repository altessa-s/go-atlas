// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clickhouse_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/orderby"
	"github.com/altessa-s/go-atlas/data/orderby/translators/clickhouse"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// q quotes an identifier part the way this dialect does.
func q(s string) string { return "`" + s + "`" }

func mustTranslator(tb testing.TB, opts ...orderby.TranslatorOption) *clickhouse.Translator {
	tb.Helper()
	tr, err := clickhouse.NewTranslator(opts...)
	require.NoError(tb, err)
	return tr
}

func TestTranslate(t *testing.T) {
	t.Parallel()
	mapping := orderby.WithFieldMapping(map[string]string{
		"createdAt": "created_at",
		"city":      "addr.city",
		"expr":      "lower(name)",
		"mapKey":    "attrs['x']",
		"subcolumn": "data.x.y",
		"deep":      "a.b.c",
		"quoted":    q("x"),
		"long":      strings.Repeat("c", 64),
	})
	for _, tc := range []struct {
		name, in, want string
		err            error
	}{
		{name: "multi", in: "create_time desc, slug", want: q("create_time") + " DESC, " + q("slug") + " ASC"},
		{name: "mapped", in: "createdAt desc", want: q("created_at") + " DESC"},
		{name: "qualified", in: "city", want: q("addr") + "." + q("city") + " ASC"},
		{name: "dotted field", in: "address.city desc", want: q("address") + "." + q("city") + " DESC"},
		{name: "case kept", in: "updatedAt", want: q("updatedAt") + " ASC"},
		{name: "expression", in: "expr", err: orderby.ErrInvalidFieldPath},
		{name: "map key", in: "mapKey", err: orderby.ErrInvalidFieldPath},
		{name: "json subcolumn", in: "subcolumn", err: orderby.ErrInvalidFieldPath},
		{name: "three parts", in: "deep", err: orderby.ErrInvalidFieldPath},
		{name: "quote", in: "quoted", err: orderby.ErrInvalidFieldPath},
		{name: "too long", in: "long", err: orderby.ErrInvalidFieldPath},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := mustTranslator(t, mapping).Translate(testhelpers.MustParseOrderBy(t, tc.in))
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
				require.Empty(t, got)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestTranslate_Empty(t *testing.T) {
	t.Parallel()
	got, err := mustTranslator(t).Translate(orderby.Spec{})
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestTranslate_AllowList(t *testing.T) {
	t.Parallel()
	tr := mustTranslator(t, orderby.WithAllowedFields("slug"), orderby.WithUntrustedInput())
	_, err := tr.Translate(testhelpers.MustParseOrderBy(t, "slug, secret desc"))
	require.ErrorIs(t, err, orderby.ErrFieldNotAllowed)
	got, err := tr.Translate(testhelpers.MustParseOrderBy(t, "slug desc"))
	require.NoError(t, err)
	require.Equal(t, q("slug")+" DESC", got)

	_, err = clickhouse.NewTranslator(orderby.WithUntrustedInput())
	require.ErrorIs(t, err, orderby.ErrAllowlistRequired)
}
