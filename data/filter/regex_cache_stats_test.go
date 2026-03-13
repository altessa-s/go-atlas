// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/filter"
)

func TestRegexCacheStats(t *testing.T) {
	// Reset counters to isolate from other tests.
	filter.ResetRegexCacheStats()

	stats := filter.RegexCacheStatsSnapshot()
	if stats.Hits != 0 || stats.Misses != 0 {
		t.Fatalf("fresh stats not zero: %+v", stats)
	}
	if stats.HitRate() != 0 {
		t.Errorf("HitRate() = %f, want 0", stats.HitRate())
	}
	if stats.TotalLookups() != 0 {
		t.Errorf("TotalLookups() = %d, want 0", stats.TotalLookups())
	}
}

func TestRegexCacheStats_ViaEvaluator(t *testing.T) {
	filter.ResetRegexCacheStats()

	// Evaluate a filter with matches() to trigger regex compilation.
	p := newTestParser(t)
	node, err := p.Parse(t.Context(), `name.matches("^test.*$")`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	data := map[string]any{"name": "test123"}
	ev := filter.NewEvaluator()
	if _, err := ev.Evaluate(node, data); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	stats := filter.RegexCacheStatsSnapshot()
	// First call is a miss (compilation required).
	if stats.Misses < 1 {
		t.Errorf("Misses = %d after first matches(), want >= 1", stats.Misses)
	}
	initialSize := stats.Size

	// Second evaluation with the same pattern should be a hit.
	if _, err := ev.Evaluate(node, data); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	stats = filter.RegexCacheStatsSnapshot()
	if stats.Hits < 1 {
		t.Errorf("Hits = %d after second matches(), want >= 1", stats.Hits)
	}
	if stats.HitRate() <= 0 {
		t.Errorf("HitRate() = %f, want > 0", stats.HitRate())
	}
	if stats.Size < initialSize {
		t.Errorf("Size = %d, want >= %d", stats.Size, initialSize)
	}
}

func TestRegexCacheStats_Reset(t *testing.T) {
	filter.ResetRegexCacheStats()

	stats := filter.RegexCacheStatsSnapshot()
	if stats.Hits != 0 || stats.Misses != 0 {
		t.Errorf("stats not zero after reset: %+v", stats)
	}
}

func TestRegexCacheStats_HitRate(t *testing.T) {
	s := filter.RegexCacheStats{Hits: 8, Misses: 2}
	if got := s.HitRate(); got != 0.8 {
		t.Errorf("HitRate() = %f, want 0.8", got)
	}
	if got := s.TotalLookups(); got != 10 {
		t.Errorf("TotalLookups() = %d, want 10", got)
	}
}
