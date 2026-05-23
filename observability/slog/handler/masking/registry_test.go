// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package masking_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/slog/handler/masking"
)

func TestRegistry(t *testing.T) {
	t.Parallel()

	t.Run("register and get", func(t *testing.T) {
		t.Parallel()

		// Test that built-in masks are registered
		fn, ok := masking.Get("email")
		require.True(t, ok, "email mask should be registered")
		require.NotNil(t, fn)

		result := fn("user@example.com")
		require.Contains(t, result, "@example.com")
		require.NotContains(t, result, "user@")
	})

	t.Run("list registered masks", func(t *testing.T) {
		t.Parallel()

		masks := masking.List()
		require.Contains(t, masks, "email")
		require.Contains(t, masks, "phone")
		require.Contains(t, masks, "url")
		require.Contains(t, masks, "s3_url")
		require.Contains(t, masks, "credit_card")
		require.Contains(t, masks, "smart")
		require.Contains(t, masks, "full")
	})

	t.Run("aliases", func(t *testing.T) {
		t.Parallel()

		// Test that aliases work
		ccFn, ok := masking.Get("cc")
		require.True(t, ok, "cc alias should be registered")

		creditCardFn, ok := masking.Get("credit_card")
		require.True(t, ok)

		// Both should produce the same result
		testCard := "4111111111111111"
		require.Equal(t, creditCardFn(testCard), ccFn(testCard))
	})

	t.Run("unknown mask", func(t *testing.T) {
		t.Parallel()

		_, ok := masking.Get("unknown_mask_type")
		require.False(t, ok)
	})
}

func TestMaskFactories(t *testing.T) {
	t.Parallel()

	t.Run("partial factory", func(t *testing.T) {
		t.Parallel()

		factory, ok := masking.GetFactory("partial")
		require.True(t, ok, "partial factory should be registered")

		params := map[string]any{
			"showFirst": 2,
			"showLast":  2,
			"maskChar":  "X",
		}

		fn, err := factory(params)
		require.NoError(t, err)

		result := fn("secret123")
		require.Equal(t, "seXXXXX23", result)
	})

	t.Run("fixed factory", func(t *testing.T) {
		t.Parallel()

		factory, ok := masking.GetFactory("fixed")
		require.True(t, ok, "fixed factory should be registered")

		params := map[string]any{
			"value": "[REDACTED]",
		}

		fn, err := factory(params)
		require.NoError(t, err)

		result := fn("anything")
		require.Equal(t, "[REDACTED]", result)
	})

	t.Run("fixed factory missing value", func(t *testing.T) {
		t.Parallel()

		factory, ok := masking.GetFactory("fixed")
		require.True(t, ok)

		params := map[string]any{}

		_, err := factory(params)
		require.Error(t, err)
		require.Contains(t, err.Error(), "fixed mask requires 'value' parameter")
	})

	t.Run("pattern factory", func(t *testing.T) {
		t.Parallel()

		factory, ok := masking.GetFactory("pattern")
		require.True(t, ok, "pattern factory should be registered")

		params := map[string]any{
			"pattern":     `\d+`,
			"replacement": "XXX",
		}

		fn, err := factory(params)
		require.NoError(t, err)

		result := fn("order-12345-abc")
		require.Equal(t, "order-XXX-abc", result)
	})

	t.Run("hash factory", func(t *testing.T) {
		t.Parallel()

		factory, ok := masking.GetFactory("hash")
		require.True(t, ok, "hash factory should be registered")

		params := map[string]any{
			"prefix": "user:",
		}

		fn, err := factory(params)
		require.NoError(t, err)

		result := fn("secret")
		require.True(t, len(result) > 5)
		require.Contains(t, result, "user:")
	})

	t.Run("list factories", func(t *testing.T) {
		t.Parallel()

		factories := masking.ListFactories()
		require.Contains(t, factories, "partial")
		require.Contains(t, factories, "fixed")
		require.Contains(t, factories, "pattern")
		require.Contains(t, factories, "hash")
		require.Contains(t, factories, "cached_partial")
	})
}

func TestCreateMask(t *testing.T) {
	t.Parallel()

	t.Run("simple mask without params", func(t *testing.T) {
		t.Parallel()

		fn, err := masking.CreateMask("email", nil)
		require.NoError(t, err)
		require.NotNil(t, fn)

		result := fn("user@example.com")
		require.Contains(t, result, "@example.com")
	})

	t.Run("factory mask with params", func(t *testing.T) {
		t.Parallel()

		params := map[string]any{
			"showFirst": 1,
			"showLast":  1,
		}

		fn, err := masking.CreateMask("partial", params)
		require.NoError(t, err)
		require.NotNil(t, fn)

		result := fn("secret")
		require.Equal(t, "s****t", result)
	})

	t.Run("simple mask rejects params", func(t *testing.T) {
		t.Parallel()

		params := map[string]any{
			"unexpected": "param",
		}

		_, err := masking.CreateMask("email", params)
		require.Error(t, err)
		require.Contains(t, err.Error(), "does not accept parameters")
	})

	t.Run("unknown mask type", func(t *testing.T) {
		t.Parallel()

		_, err := masking.CreateMask("unknown", nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unknown mask type")
	})
}
