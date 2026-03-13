// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package strings_test

import (
	"strings"
	"testing"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

func TestNewInterner(t *testing.T) {
	interner := corestrings.NewInterner(100)
	if interner == nil {
		t.Fatal("NewInterner() returned nil")
	}
	if interner.MaxSize() != 100 {
		t.Errorf("MaxSize() = %d, want 100", interner.MaxSize())
	}
	if !interner.IsEmpty() {
		t.Error("new interner should be empty")
	}
}

func TestInterner_String(t *testing.T) {
	interner := corestrings.NewInterner(100)

	s1 := interner.String("hello")
	s2 := interner.String("hello")
	if s1 != s2 {
		t.Error("String() should return same string for same input")
	}
	if interner.IsEmpty() {
		t.Error("interner should not be empty after String()")
	}
}

func TestInterner_UpperString(t *testing.T) {
	interner := corestrings.NewInterner(100)
	got := interner.UpperString("hello")
	if got != "HELLO" {
		t.Errorf("UpperString(hello) = %q, want HELLO", got)
	}
}

func TestInterner_LowerString(t *testing.T) {
	interner := corestrings.NewInterner(100)
	got := interner.LowerString("HELLO")
	if got != "hello" {
		t.Errorf("LowerString(HELLO) = %q, want hello", got)
	}
}

func TestInterner_TrimString(t *testing.T) {
	interner := corestrings.NewInterner(100)
	got := interner.TrimString("  hello  ")
	if got != "hello" {
		t.Errorf("TrimString() = %q, want hello", got)
	}
}

func TestInterner_PrefixString(t *testing.T) {
	interner := corestrings.NewInterner(100)
	got := interner.PrefixString("world", "hello-")
	if got != "hello-world" {
		t.Errorf("PrefixString() = %q, want hello-world", got)
	}
}

func TestInterner_SuffixString(t *testing.T) {
	interner := corestrings.NewInterner(100)
	got := interner.SuffixString("hello", "-world")
	if got != "hello-world" {
		t.Errorf("SuffixString() = %q, want hello-world", got)
	}
}

func TestInterner_WrapString(t *testing.T) {
	interner := corestrings.NewInterner(100)
	got := interner.WrapString("hello", "[", "]")
	if got != "[hello]" {
		t.Errorf("WrapString() = %q, want [hello]", got)
	}
}

func TestInterner_CleanPathString(t *testing.T) {
	interner := corestrings.NewInterner(100)
	got := interner.CleanPathString("/a//b/")
	if got != "/a/b" {
		t.Errorf("CleanPathString() = %q, want /a/b", got)
	}
}

func TestInterner_StringSlice(t *testing.T) {
	interner := corestrings.NewInterner(100)
	input := []string{"a", "b", "c"}
	got := interner.StringSlice(input)
	if len(got) != 3 {
		t.Fatalf("StringSlice() len = %d, want 3", len(got))
	}
	for i, v := range input {
		if got[i] != v {
			t.Errorf("StringSlice()[%d] = %q, want %q", i, got[i], v)
		}
	}
}

func TestInterner_StringMap(t *testing.T) {
	interner := corestrings.NewInterner(100)
	input := map[string]string{"k": "v"}
	got := interner.StringMap(input)
	if got["k"] != "v" {
		t.Errorf("StringMap()[k] = %q, want v", got["k"])
	}
}

func TestInterner_FormatString(t *testing.T) {
	interner := corestrings.NewInterner(100)
	got := interner.FormatString("hello %s", "world")
	if got != "hello world" {
		t.Errorf("FormatString() = %q, want 'hello world'", got)
	}
}

func TestInterner_JoinString(t *testing.T) {
	interner := corestrings.NewInterner(100)
	got := interner.JoinString([]string{"a", "b", "c"}, ",")
	if got != "a,b,c" {
		t.Errorf("JoinString() = %q, want a,b,c", got)
	}
}

func TestInterner_JoinWith(t *testing.T) {
	interner := corestrings.NewInterner(100)
	got := interner.JoinWith([]string{"a", "b"}, func(parts []string) string {
		return strings.Join(parts, "-")
	})
	if got != "a-b" {
		t.Errorf("JoinWith() = %q, want a-b", got)
	}
}

func TestInterner_SizeAndLoadFactor(t *testing.T) {
	interner := corestrings.NewInterner(10)

	if interner.Size() != 0 {
		t.Errorf("Size() = %d, want 0", interner.Size())
	}
	if interner.LoadFactor() != 0 {
		t.Errorf("LoadFactor() = %f, want 0", interner.LoadFactor())
	}

	interner.String("a")
	if interner.Size() < 1 {
		t.Errorf("Size() = %d after insert, want >= 1", interner.Size())
	}
	if interner.IsFull() && interner.Size() < interner.MaxSize() {
		t.Error("IsFull() should be false when not full")
	}
}

func TestInterner_Reset(t *testing.T) {
	interner := corestrings.NewInterner(100)
	interner.String("hello")
	interner.Reset()
	if !interner.IsEmpty() {
		t.Error("IsEmpty() should be true after Reset()")
	}
}

func TestGlobalInterner(t *testing.T) {
	t.Cleanup(corestrings.ResetGlobalInterner)

	g := corestrings.GlobalInterner()
	if g == nil {
		t.Fatal("GlobalInterner() returned nil")
	}
}

func TestResetGlobalInterner(t *testing.T) {
	t.Cleanup(corestrings.ResetGlobalInterner)

	corestrings.InternString("leak-check")
	g := corestrings.GlobalInterner()
	if g.IsEmpty() {
		t.Fatal("global interner should not be empty after InternString")
	}

	corestrings.ResetGlobalInterner()
	if !g.IsEmpty() {
		t.Error("global interner should be empty after ResetGlobalInterner")
	}
}

func TestInterner_Stats(t *testing.T) {
	interner := corestrings.NewInterner(100)

	// Initially all counters are zero.
	stats := interner.Stats()
	if stats.HotHits != 0 || stats.ColdHits != 0 || stats.Misses != 0 || stats.Evictions != 0 {
		t.Fatalf("fresh interner stats not zero: %+v", stats)
	}
	if stats.TotalLookups() != 0 {
		t.Errorf("TotalLookups() = %d, want 0", stats.TotalLookups())
	}
	if stats.HitRate() != 0 {
		t.Errorf("HitRate() = %f, want 0", stats.HitRate())
	}

	// First lookup is a miss.
	interner.String("hello")
	stats = interner.Stats()
	if stats.Misses != 1 {
		t.Errorf("Misses = %d after first String(), want 1", stats.Misses)
	}

	// Second lookup for the same string is a cold hit.
	interner.String("hello")
	stats = interner.Stats()
	if stats.ColdHits < 1 {
		t.Errorf("ColdHits = %d after second String(), want >= 1", stats.ColdHits)
	}

	// Hit rate should be > 0.
	if stats.HitRate() <= 0 {
		t.Errorf("HitRate() = %f, want > 0", stats.HitRate())
	}
	if stats.TotalLookups() != stats.HotHits+stats.ColdHits+stats.Misses {
		t.Errorf("TotalLookups() inconsistent: %d != %d+%d+%d",
			stats.TotalLookups(), stats.HotHits, stats.ColdHits, stats.Misses)
	}

	// MaxSize and CurrentSize should be populated.
	if stats.MaxSize != 100 {
		t.Errorf("MaxSize = %d, want 100", stats.MaxSize)
	}
	if stats.CurrentSize < 1 {
		t.Errorf("CurrentSize = %d, want >= 1", stats.CurrentSize)
	}
}

func TestInterner_ResetStats(t *testing.T) {
	interner := corestrings.NewInterner(100)
	interner.String("foo")
	interner.String("foo")

	interner.ResetStats()
	stats := interner.Stats()
	if stats.HotHits != 0 || stats.ColdHits != 0 || stats.Misses != 0 || stats.Evictions != 0 {
		t.Errorf("stats not zero after ResetStats: %+v", stats)
	}

	// Cache entries should still exist.
	if interner.IsEmpty() {
		t.Error("interner should not be empty after ResetStats")
	}
}

func TestInterner_StatsAfterReset(t *testing.T) {
	interner := corestrings.NewInterner(100)
	interner.String("bar")
	interner.Reset()

	stats := interner.Stats()
	if stats.HotHits != 0 || stats.ColdHits != 0 || stats.Misses != 0 {
		t.Errorf("stats not zero after Reset: %+v", stats)
	}
}

func TestInternerStats_HitRate(t *testing.T) {
	s := corestrings.InternerStats{HotHits: 3, ColdHits: 5, Misses: 2}
	if got := s.HitRate(); got != 0.8 {
		t.Errorf("HitRate() = %f, want 0.8", got)
	}
	if got := s.HotHitRate(); got != 0.3 {
		t.Errorf("HotHitRate() = %f, want 0.3", got)
	}
	if got := s.TotalLookups(); got != 10 {
		t.Errorf("TotalLookups() = %d, want 10", got)
	}
}

func TestInternerStats_HitRateZero(t *testing.T) {
	s := corestrings.InternerStats{}
	if got := s.HitRate(); got != 0 {
		t.Errorf("HitRate() = %f, want 0", got)
	}
	if got := s.HotHitRate(); got != 0 {
		t.Errorf("HotHitRate() = %f, want 0", got)
	}
}

func TestInternGlobalFunctions(t *testing.T) {
	t.Cleanup(corestrings.ResetGlobalInterner)

	if got := corestrings.InternString("test"); got != "test" {
		t.Errorf("InternString() = %q", got)
	}
	if got := corestrings.InternLowerString("TEST"); got != "test" {
		t.Errorf("InternLowerString() = %q", got)
	}
	if got := corestrings.InternUpperString("test"); got != "TEST" {
		t.Errorf("InternUpperString() = %q", got)
	}
	if got := corestrings.InternTrimString("  x  "); got != "x" {
		t.Errorf("InternTrimString() = %q", got)
	}
	if got := corestrings.InternCleanPathString("/a//b/"); got != "/a/b" {
		t.Errorf("InternCleanPathString() = %q", got)
	}
	if got := corestrings.InternPrefixString("b", "a-"); got != "a-b" {
		t.Errorf("InternPrefixString() = %q", got)
	}
	if got := corestrings.InternSuffixString("a", "-b"); got != "a-b" {
		t.Errorf("InternSuffixString() = %q", got)
	}
	if got := corestrings.InternWrapString("x", "(", ")"); got != "(x)" {
		t.Errorf("InternWrapString() = %q", got)
	}
	if got := corestrings.InternStringSlice([]string{"a"}); len(got) != 1 || got[0] != "a" {
		t.Errorf("InternStringSlice() = %v", got)
	}
	if got := corestrings.InternStringMap(map[string]string{"k": "v"}); got["k"] != "v" {
		t.Errorf("InternStringMap() = %v", got)
	}
	if got := corestrings.InternFormatString("%d", 42); got != "42" {
		t.Errorf("InternFormatString() = %q", got)
	}
	if got := corestrings.InternJoinString([]string{"a", "b"}, "-"); got != "a-b" {
		t.Errorf("InternJoinString() = %q", got)
	}
	if got := corestrings.InternJoinWith([]string{"a", "b"}, func(p []string) string { return strings.Join(p, "+") }); got != "a+b" {
		t.Errorf("InternJoinWith() = %q", got)
	}
}
