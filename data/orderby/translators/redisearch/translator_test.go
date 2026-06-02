// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisearch_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/orderby"
	"github.com/altessa-s/go-atlas/data/orderby/translators/redisearch"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func mustTranslator(tb testing.TB, opts ...orderby.TranslatorOption) *redisearch.Translator {
	tb.Helper()
	tr, err := redisearch.NewTranslator(opts...)
	require.NoError(tb, err)
	return tr
}

func TestTranslate_SingleKey(t *testing.T) {
	t.Parallel()
	ob := testhelpers.MustParseOrderBy(t, "createdAt desc")

	got, err := mustTranslator(t).Translate(ob)
	require.NoError(t, err)
	require.Equal(t, redisearch.SortBy{Field: "createdAt", Descending: true}, got)
}

func TestTranslate_DefaultAscending(t *testing.T) {
	t.Parallel()
	ob := testhelpers.MustParseOrderBy(t, "createdAt")

	got, err := mustTranslator(t).Translate(ob)
	require.NoError(t, err)
	require.Equal(t, redisearch.SortBy{Field: "createdAt", Descending: false}, got)
}

func TestTranslate_Empty(t *testing.T) {
	t.Parallel()
	got, err := mustTranslator(t).Translate(orderby.Spec{})
	require.NoError(t, err)
	require.Equal(t, redisearch.SortBy{}, got)
}

func TestTranslate_FieldMapping(t *testing.T) {
	t.Parallel()
	ob := testhelpers.MustParseOrderBy(t, "createdAt desc")

	got, err := mustTranslator(t, orderby.WithFieldMapping(map[string]string{
		"createdAt": "created_at",
	})).Translate(ob)
	require.NoError(t, err)
	require.Equal(t, redisearch.SortBy{Field: "created_at", Descending: true}, got)
}

func TestTranslate_TooManyKeys(t *testing.T) {
	t.Parallel()
	ob := testhelpers.MustParseOrderBy(t, "a, b desc")

	_, err := mustTranslator(t).Translate(ob)
	require.ErrorIs(t, err, orderby.ErrTooManySortKeys)
}

func TestTranslate_AllowedFields(t *testing.T) {
	t.Parallel()
	ob := testhelpers.MustParseOrderBy(t, "secret desc")

	_, err := mustTranslator(t, orderby.WithAllowedFields("createdAt")).Translate(ob)
	require.ErrorIs(t, err, orderby.ErrFieldNotAllowed)
}

// TestNewTranslator_UntrustedInput_RequiresAllowlist locks the contract
// that misconfiguration is rejected at construction.
func TestNewTranslator_UntrustedInput_RequiresAllowlist(t *testing.T) {
	t.Parallel()

	_, err := redisearch.NewTranslator(orderby.WithUntrustedInput())
	require.ErrorIs(t, err, orderby.ErrAllowlistRequired)
}

// TestTranslate_NestedField asserts a dotted field path round-trips
// unchanged through the RediSearch translator. RediSearch accepts a
// nested attribute name in SORTBY as long as the underlying index
// declares the JSONPath, which is the caller's concern.
func TestTranslate_NestedField(t *testing.T) {
	t.Parallel()
	ob := testhelpers.MustParseOrderBy(t, "address.city desc")

	got, err := mustTranslator(t, orderby.WithAllowedFields("address.*")).Translate(ob)
	require.NoError(t, err)
	require.Equal(t, redisearch.SortBy{Field: "address.city", Descending: true}, got)
}
