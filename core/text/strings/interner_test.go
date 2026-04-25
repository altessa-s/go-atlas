// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package strings_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

func TestNewInterner(t *testing.T) {
	interner := corestrings.NewInterner(100)
	require.NotNil(t, interner)
	require.Equal(t, int64(100), interner.MaxSize())
	require.True(t, interner.IsEmpty(), "new interner should be empty")
}

func TestInterner_String(t *testing.T) {
	interner := corestrings.NewInterner(100)

	s1 := interner.String("hello")
	s2 := interner.String("hello")
	require.Equal(t, s1, s2, "String() should return same string for same input")
	require.False(t, interner.IsEmpty(), "interner should not be empty after String()")
}

func TestInterner_UpperString(t *testing.T) {
	interner := corestrings.NewInterner(100)
	require.Equal(t, "HELLO", interner.UpperString("hello"))
}

func TestInterner_LowerString(t *testing.T) {
	interner := corestrings.NewInterner(100)
	require.Equal(t, "hello", interner.LowerString("HELLO"))
}

func TestInterner_TrimString(t *testing.T) {
	interner := corestrings.NewInterner(100)
	require.Equal(t, "hello", interner.TrimString("  hello  "))
}

func TestInterner_PrefixString(t *testing.T) {
	interner := corestrings.NewInterner(100)
	require.Equal(t, "hello-world", interner.PrefixString("world", "hello-"))
}

func TestInterner_SuffixString(t *testing.T) {
	interner := corestrings.NewInterner(100)
	require.Equal(t, "hello-world", interner.SuffixString("hello", "-world"))
}

func TestInterner_WrapString(t *testing.T) {
	interner := corestrings.NewInterner(100)
	require.Equal(t, "[hello]", interner.WrapString("hello", "[", "]"))
}

func TestInterner_CleanPathString(t *testing.T) {
	interner := corestrings.NewInterner(100)
	require.Equal(t, "/a/b", interner.CleanPathString("/a//b/"))
}

func TestInterner_StringSlice(t *testing.T) {
	interner := corestrings.NewInterner(100)
	input := []string{"a", "b", "c"}
	got := interner.StringSlice(input)
	require.Equal(t, input, got)
}

func TestInterner_StringMap(t *testing.T) {
	interner := corestrings.NewInterner(100)
	input := map[string]string{"k": "v"}
	got := interner.StringMap(input)
	require.Equal(t, "v", got["k"])
}

func TestInterner_FormatString(t *testing.T) {
	interner := corestrings.NewInterner(100)
	require.Equal(t, "hello world", interner.FormatString("hello %s", "world"))
}

func TestInterner_JoinString(t *testing.T) {
	interner := corestrings.NewInterner(100)
	require.Equal(t, "a,b,c", interner.JoinString([]string{"a", "b", "c"}, ","))
}

func TestInterner_JoinWith(t *testing.T) {
	interner := corestrings.NewInterner(100)
	got := interner.JoinWith([]string{"a", "b"}, func(parts []string) string {
		return strings.Join(parts, "-")
	})
	require.Equal(t, "a-b", got)
}

func TestInterner_SizeAndLoadFactor(t *testing.T) {
	interner := corestrings.NewInterner(10)

	require.Equal(t, int64(0), interner.Size())
	require.Equal(t, float64(0), interner.LoadFactor())

	interner.String("a")
	require.GreaterOrEqual(t, interner.Size(), int64(1))
	if interner.IsFull() {
		require.GreaterOrEqual(t, interner.Size(), interner.MaxSize())
	}
}

func TestInterner_Reset(t *testing.T) {
	interner := corestrings.NewInterner(100)
	interner.String("hello")
	interner.Reset()
	require.True(t, interner.IsEmpty(), "IsEmpty() should be true after Reset()")
}

func TestGlobalInterner(t *testing.T) {
	t.Cleanup(corestrings.ResetGlobalInterner)

	g := corestrings.GlobalInterner()
	require.NotNil(t, g)
}

func TestResetGlobalInterner(t *testing.T) {
	t.Cleanup(corestrings.ResetGlobalInterner)

	corestrings.InternString("leak-check")
	g := corestrings.GlobalInterner()
	require.False(t, g.IsEmpty(), "global interner should not be empty after InternString")

	corestrings.ResetGlobalInterner()
	require.True(t, g.IsEmpty(), "global interner should be empty after ResetGlobalInterner")
}

func TestInterner_Stats(t *testing.T) {
	interner := corestrings.NewInterner(100)

	// Initially all counters are zero.
	stats := interner.Stats()
	require.Equal(t, uint64(0), stats.HotHits)
	require.Equal(t, uint64(0), stats.ColdHits)
	require.Equal(t, uint64(0), stats.Misses)
	require.Equal(t, uint64(0), stats.Evictions)
	require.Equal(t, uint64(0), stats.TotalLookups())
	require.Equal(t, float64(0), stats.HitRate())

	// First lookup is a miss.
	interner.String("hello")
	stats = interner.Stats()
	require.Equal(t, uint64(1), stats.Misses)

	// Second lookup for the same string is a cold hit.
	interner.String("hello")
	stats = interner.Stats()
	require.GreaterOrEqual(t, stats.ColdHits, uint64(1))

	// Hit rate should be > 0.
	require.Greater(t, stats.HitRate(), float64(0))
	require.Equal(t, stats.HotHits+stats.ColdHits+stats.Misses, stats.TotalLookups())

	// MaxSize and CurrentSize should be populated.
	require.Equal(t, int64(100), stats.MaxSize)
	require.GreaterOrEqual(t, stats.CurrentSize, int64(1))
}

func TestInterner_ResetStats(t *testing.T) {
	interner := corestrings.NewInterner(100)
	interner.String("foo")
	interner.String("foo")

	interner.ResetStats()
	stats := interner.Stats()
	require.Equal(t, uint64(0), stats.HotHits)
	require.Equal(t, uint64(0), stats.ColdHits)
	require.Equal(t, uint64(0), stats.Misses)
	require.Equal(t, uint64(0), stats.Evictions)

	// Cache entries should still exist.
	require.False(t, interner.IsEmpty(), "interner should not be empty after ResetStats")
}

func TestInterner_StatsAfterReset(t *testing.T) {
	interner := corestrings.NewInterner(100)
	interner.String("bar")
	interner.Reset()

	stats := interner.Stats()
	require.Equal(t, uint64(0), stats.HotHits)
	require.Equal(t, uint64(0), stats.ColdHits)
	require.Equal(t, uint64(0), stats.Misses)
}

func TestInternerStats_HitRate(t *testing.T) {
	s := corestrings.InternerStats{HotHits: 3, ColdHits: 5, Misses: 2}
	require.Equal(t, 0.8, s.HitRate())
	require.Equal(t, 0.3, s.HotHitRate())
	require.Equal(t, uint64(10), s.TotalLookups())
}

func TestInternerStats_HitRateZero(t *testing.T) {
	s := corestrings.InternerStats{}
	require.Equal(t, float64(0), s.HitRate())
	require.Equal(t, float64(0), s.HotHitRate())
}

func TestInternGlobalFunctions(t *testing.T) {
	t.Cleanup(corestrings.ResetGlobalInterner)

	require.Equal(t, "test", corestrings.InternString("test"))
	require.Equal(t, "test", corestrings.InternLowerString("TEST"))
	require.Equal(t, "TEST", corestrings.InternUpperString("test"))
	require.Equal(t, "x", corestrings.InternTrimString("  x  "))
	require.Equal(t, "/a/b", corestrings.InternCleanPathString("/a//b/"))
	require.Equal(t, "a-b", corestrings.InternPrefixString("b", "a-"))
	require.Equal(t, "a-b", corestrings.InternSuffixString("a", "-b"))
	require.Equal(t, "(x)", corestrings.InternWrapString("x", "(", ")"))
	require.Equal(t, []string{"a"}, corestrings.InternStringSlice([]string{"a"}))
	require.Equal(t, "v", corestrings.InternStringMap(map[string]string{"k": "v"})["k"])
	require.Equal(t, "42", corestrings.InternFormatString("%d", 42))
	require.Equal(t, "a-b", corestrings.InternJoinString([]string{"a", "b"}, "-"))
	require.Equal(t, "a+b", corestrings.InternJoinWith([]string{"a", "b"}, func(p []string) string { return strings.Join(p, "+") }))
}
