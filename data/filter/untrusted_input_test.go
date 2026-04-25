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
// CEL-from-untrusted-source vulnerability: a translator marked with
// [filter.WithUntrustedInput] must refuse to translate unless an explicit
// allowlist is configured. Without that, a hostile client could filter on
// any field the storage layer indexes (e.g. `passwordHash > ""` to
// enumerate accounts), which is exactly what this knob exists to prevent.
func TestUntrustedInput_RequiresAllowlist(t *testing.T) {
	const safeField, expr = "name", `name == "x"`

	t.Run("mongo translator", func(t *testing.T) {
		t.Parallel()
		node := testhelpers.MustParseFilter(t, expr)

		// Untrusted + no allowlist -> rejected.
		trans := mongo.NewTranslator(filter.WithUntrustedInput())
		_, err := trans.Translate(node)
		require.ErrorIs(t, err, filter.ErrAllowlistRequired)

		// Untrusted + allowlist -> translated.
		trans = mongo.NewTranslator(filter.WithUntrustedInput(), filter.WithAllowedFields(safeField))
		_, err = trans.Translate(node)
		require.NoError(t, err)

		// Trusted (default) + no allowlist -> still permissive (backwards compat).
		trans = mongo.NewTranslator()
		_, err = trans.Translate(node)
		require.NoError(t, err)
	})

	t.Run("lua translator", func(t *testing.T) {
		t.Parallel()
		node := testhelpers.MustParseFilter(t, expr)

		trans := lua.NewTranslator("row", filter.WithUntrustedInput())
		_, err := trans.Translate(node)
		require.ErrorIs(t, err, filter.ErrAllowlistRequired)

		trans = lua.NewTranslator("row", filter.WithUntrustedInput(), filter.WithAllowedFields(safeField))
		_, err = trans.Translate(node)
		require.NoError(t, err)
	})

	t.Run("redisearch translator", func(t *testing.T) {
		t.Parallel()
		node := testhelpers.MustParseFilter(t, expr)
		schema := map[string]redisearch.FieldType{safeField: redisearch.FieldTypeText}

		trans := redisearch.NewTranslator(schema, filter.WithUntrustedInput())
		_, err := trans.Translate(node)
		require.ErrorIs(t, err, filter.ErrAllowlistRequired)

		trans = redisearch.NewTranslator(schema, filter.WithUntrustedInput(), filter.WithAllowedFields(safeField))
		_, err = trans.Translate(node)
		require.NoError(t, err)
	})

	t.Run("evaluator", func(t *testing.T) {
		t.Parallel()
		node := testhelpers.MustParseFilter(t, expr)

		eval := filter.NewEvaluator(filter.WithUntrustedInput())
		_, err := eval.Evaluate(node, map[string]any{safeField: "x"})
		require.ErrorIs(t, err, filter.ErrAllowlistRequired)

		eval = filter.NewEvaluator(filter.WithUntrustedInput(), filter.WithAllowedFields(safeField))
		ok, err := eval.Evaluate(node, map[string]any{safeField: "x"})
		require.NoError(t, err)
		require.True(t, ok)
	})
}
