// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/data/filter/translators/lua"
	"github.com/altessa-s/go-atlas/data/filter/translators/mongo"
	"github.com/altessa-s/go-atlas/data/filter/translators/redisearch"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// TestUntrustedInput_RequiresAllowlist guards against regressing the
// CEL-from-untrusted-source vulnerability: a translator or evaluator
// marked with [filter.WithUntrustedInput] must refuse construction
// unless an explicit allow-list is also configured. Without that, a
// hostile client could filter on any field the storage layer indexes
// (e.g. `passwordHash > ""` to enumerate accounts), which is exactly
// what this knob exists to prevent.
//
// The check happens in [filter.NewTranslatorContext] (and therefore in
// every translator/evaluator constructor) so misconfiguration surfaces
// at process start rather than on the first untrusted query.
func TestUntrustedInput_RequiresAllowlist(t *testing.T) {
	const safeField, expr = "name", `name == "x"`

	t.Run("mongo translator", func(t *testing.T) {
		t.Parallel()
		node := testhelpers.MustParseFilter(t, expr)

		// Untrusted + no allow-list -> constructor rejects.
		_, err := mongo.NewTranslator(filter.WithUntrustedInput())
		require.ErrorIs(t, err, filter.ErrAllowlistRequired)

		// Untrusted + allow-list -> constructs and translates.
		trans, err := mongo.NewTranslator(filter.WithUntrustedInput(), filter.WithAllowedFields(safeField))
		require.NoError(t, err)
		_, err = trans.Translate(node)
		require.NoError(t, err)

		// Trusted (default) + no allow-list -> still permissive (backwards compat).
		trans, err = mongo.NewTranslator()
		require.NoError(t, err)
		_, err = trans.Translate(node)
		require.NoError(t, err)
	})

	t.Run("lua translator", func(t *testing.T) {
		t.Parallel()
		node := testhelpers.MustParseFilter(t, expr)

		_, err := lua.NewTranslator("row", filter.WithUntrustedInput())
		require.ErrorIs(t, err, filter.ErrAllowlistRequired)

		trans, err := lua.NewTranslator("row", filter.WithUntrustedInput(), filter.WithAllowedFields(safeField))
		require.NoError(t, err)
		_, err = trans.Translate(node)
		require.NoError(t, err)
	})

	t.Run("redisearch translator", func(t *testing.T) {
		t.Parallel()
		node := testhelpers.MustParseFilter(t, expr)
		schema := map[string]redisearch.FieldType{safeField: redisearch.FieldTypeText}

		_, err := redisearch.NewTranslator(schema, filter.WithUntrustedInput())
		require.ErrorIs(t, err, filter.ErrAllowlistRequired)

		trans, err := redisearch.NewTranslator(schema, filter.WithUntrustedInput(), filter.WithAllowedFields(safeField))
		require.NoError(t, err)
		_, err = trans.Translate(node)
		require.NoError(t, err)
	})

	t.Run("evaluator", func(t *testing.T) {
		t.Parallel()
		node := testhelpers.MustParseFilter(t, expr)

		_, err := filter.NewEvaluator(filter.WithUntrustedInput())
		require.ErrorIs(t, err, filter.ErrAllowlistRequired)

		eval, err := filter.NewEvaluator(filter.WithUntrustedInput(), filter.WithAllowedFields(safeField))
		require.NoError(t, err)
		ok, err := eval.Evaluate(node, map[string]any{safeField: "x"})
		require.NoError(t, err)
		require.True(t, ok)
	})
}

// TestUntrustedInput_EmptyAllowlist guards the configuration-bug path
// where [filter.WithAllowedFields] was called with no arguments (e.g.
// an empty slice from a config loader). The check has to happen at
// construction so the bug surfaces during boot, not on the first
// untrusted query.
func TestUntrustedInput_EmptyAllowlist(t *testing.T) {
	t.Parallel()

	_, err := mongo.NewTranslator(
		filter.WithUntrustedInput(),
		filter.WithAllowedFields(),
	)
	require.ErrorIs(t, err, filter.ErrAllowlistRequired)

	var empty []string
	_, err = mongo.NewTranslator(
		filter.WithUntrustedInput(),
		filter.WithAllowedFields(empty...),
	)
	require.ErrorIs(t, err, filter.ErrAllowlistRequired)
}
