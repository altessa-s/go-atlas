// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/tracing"
)

func TestParseTraceState_Truncation(t *testing.T) {
	t.Parallel()

	t.Run("mixed_lengths_within_128_drop_trailing_entries_only", func(t *testing.T) {
		t.Parallel()
		var parts []string
		for i := range 8 {
			parts = append(parts, fmt.Sprintf("v%d=%s", i, strings.Repeat("x", 56)))
		}
		want := strings.Join(parts, ",")
		// mid overflows 512 bytes; tail is short enough that a greedy append would
		// have included tail while skipping mid.
		parts = append(parts, "mid="+strings.Repeat("m", 40), "tail=1")

		got, ok := tracing.ParseTraceState(strings.Join(parts, ","))
		require.True(t, ok)
		require.Equal(t, want, got)
	})

	t.Run("entries_over_128_chars_dropped_first_from_right", func(t *testing.T) {
		t.Parallel()
		keep1 := "keep1=1"
		bigLeft := "bigleft=" + strings.Repeat("a", 190)   // 198 chars (> 128)
		bigRight := "bigright=" + strings.Repeat("b", 190) // 199 chars (> 128)
		keep2 := "keep2=" + strings.Repeat("c", 110)       // 116 chars (<= 128)

		raw := strings.Join([]string{keep1, bigLeft, bigRight, keep2}, ",")
		require.Greater(t, len(raw), tracing.MaxTraceStateLen)

		got, ok := tracing.ParseTraceState(raw)
		require.True(t, ok)
		require.LessOrEqual(t, len(got), tracing.MaxTraceStateLen)
		require.Equal(t, strings.Join([]string{keep1, bigLeft, keep2}, ","), got)
	})

	t.Run("oversized_entries_dropped_before_trailing_short_entries", func(t *testing.T) {
		t.Parallel()
		var prefix []string
		for i := range 8 {
			prefix = append(prefix, fmt.Sprintf("s%d=%s", i, strings.Repeat("s", 56)))
		}
		huge := "huge=" + strings.Repeat("h", 150) // 155 chars (> 128)
		tail := "tail=" + strings.Repeat("t", 56)  // 61 chars (<= 128)

		// Removing huge (155) leaves 8*59 + 61 + 8 commas = 541 > 512, so tail is
		// dropped next in the trailing pass, leaving prefix[0..7].
		raw := strings.Join(append(append([]string{}, prefix...), huge, tail), ",")
		got, ok := tracing.ParseTraceState(raw)
		require.True(t, ok)
		require.Equal(t, strings.Join(prefix, ","), got)
	})

	t.Run("single_entry_over_128_kept_when_total_fits_512", func(t *testing.T) {
		t.Parallel()
		raw := "big=" + strings.Repeat("a", 200) + ",small=1"
		got, ok := tracing.ParseTraceState(raw)
		require.True(t, ok)
		require.Equal(t, raw, got)
	})
}
