// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisfilter_test

import (
	"maps"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/cache/probfilter/internal/redisfilter"
)

func TestInfoFields_Pin(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		reply any
	}{
		{"resp2 array", []any{int64(7), "skipped", "a", int64(1), "b", int64(2), "dangling"}},
		{"resp3 map", map[any]any{int64(7): "skipped", "a": int64(1), "b": int64(2)}},
		{"string map", map[string]any{"a": int64(1), "b": int64(2)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := maps.Collect(redisfilter.InfoFields(tc.reply))
			require.Equal(t, map[string]any{"a": int64(1), "b": int64(2)}, got)

			seen := 0
			for range redisfilter.InfoFields(tc.reply) {
				seen++
				break
			}
			require.Equal(t, 1, seen, "the consumer stops the iteration")
		})
	}

	t.Run("unexpected shape", func(t *testing.T) {
		t.Parallel()
		for range redisfilter.InfoFields("OK") {
			require.FailNow(t, "a scalar reply yields nothing")
		}
	})
}
