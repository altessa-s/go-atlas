// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package meili_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/orderby"
	"github.com/altessa-s/go-atlas/data/orderby/translators/meili"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func mustTranslator(tb testing.TB, opts ...orderby.TranslatorOption) *meili.Translator {
	tb.Helper()
	tr, err := meili.NewTranslator(opts...)
	require.NoError(tb, err)
	return tr
}

func TestTranslate_Multi(t *testing.T) {
	t.Parallel()
	ob := testhelpers.MustParseOrderBy(t, "createdAt desc, name")

	got, err := mustTranslator(t).Translate(ob)
	require.NoError(t, err)
	require.Equal(t, []string{"createdAt:desc", "name:asc"}, got)
}

func TestTranslate_Empty(t *testing.T) {
	t.Parallel()
	got, err := mustTranslator(t).Translate(orderby.Spec{})
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestTranslate_FieldMapping(t *testing.T) {
	t.Parallel()
	ob := testhelpers.MustParseOrderBy(t, "createdAt desc")

	got, err := mustTranslator(t, orderby.WithFieldMapping(map[string]string{
		"createdAt": "created_at",
	})).Translate(ob)
	require.NoError(t, err)
	require.Equal(t, []string{"created_at:desc"}, got)
}

func TestTranslate_AllowedFields(t *testing.T) {
	t.Parallel()
	ob := testhelpers.MustParseOrderBy(t, "createdAt desc, secret")

	_, err := mustTranslator(t, orderby.WithAllowedFields("createdAt")).Translate(ob)
	require.ErrorIs(t, err, orderby.ErrFieldNotAllowed)
}

// TestNewTranslator_UntrustedInput_RequiresAllowlist locks the contract
// that misconfiguration is rejected at construction.
func TestNewTranslator_UntrustedInput_RequiresAllowlist(t *testing.T) {
	t.Parallel()

	_, err := meili.NewTranslator(orderby.WithUntrustedInput())
	require.ErrorIs(t, err, orderby.ErrAllowlistRequired)
}

// TestTranslate_NestedField asserts that dotted field paths round-trip
// through the Meili translator unchanged — Meilisearch accepts nested
// attributes in sort instructions natively.
func TestTranslate_NestedField(t *testing.T) {
	t.Parallel()
	ob := testhelpers.MustParseOrderBy(t, "address.city desc, address.zip.code asc")

	got, err := mustTranslator(t, orderby.WithAllowedFields("address.*")).Translate(ob)
	require.NoError(t, err)
	require.Equal(t, []string{"address.city:desc", "address.zip.code:asc"}, got)
}
