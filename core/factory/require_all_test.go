// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"strings"
	"testing"
)

// TestRequireAllDependencies_AggregatesAllMissing asserts that every missing
// dependency is reported, not just the first one observed during map
// iteration. Before this fix, callers had to re-run validation repeatedly to
// surface each missing dep in turn.
func TestRequireAllDependencies_AggregatesAllMissing(t *testing.T) {
	b := NewBase(nil)

	err := b.RequireAllDependencies(map[string]any{
		"database": nil,
		"cache":    nil,
		"logger":   "present",
		"metrics":  nil,
	})
	if err == nil {
		t.Fatal("expected aggregated error, got nil")
	}

	msg := err.Error()
	for _, name := range []string{"database", "cache", "metrics"} {
		if !strings.Contains(msg, name+" is required") {
			t.Errorf("aggregated error missing %q: %s", name, msg)
		}
	}
	if strings.Contains(msg, "logger is required") {
		t.Errorf("aggregated error unexpectedly reports present dep: %s", msg)
	}
}

// TestRequireAllDependencies_AllPresentReturnsNil is a sanity check that the
// happy path still returns nil after the switch from first-error to
// errors.Join.
func TestRequireAllDependencies_AllPresentStillNil(t *testing.T) {
	b := NewBase(nil)
	err := b.RequireAllDependencies(map[string]any{
		"a": "val",
		"b": 42,
		"c": struct{}{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// BenchmarkRequireAllDependencies_AllPresent is the hot path: construction
// time, all deps satisfied. Measures the cost added by the new slice +
// errors.Join compared to the old first-error return.
func BenchmarkRequireAllDependencies_AllPresent(b *testing.B) {
	base := NewBase(nil)
	deps := map[string]any{
		"db":      "x",
		"cache":   "x",
		"logger":  "x",
		"metrics": "x",
		"tracer":  "x",
	}
	b.ReportAllocs()
	for b.Loop() {
		_ = base.RequireAllDependencies(deps)
	}
}

// BenchmarkRequireAllDependencies_AllMissing measures the cold path where
// every dep is nil and errors.Join must allocate.
func BenchmarkRequireAllDependencies_AllMissing(b *testing.B) {
	base := NewBase(nil)
	deps := map[string]any{
		"db":      nil,
		"cache":   nil,
		"logger":  nil,
		"metrics": nil,
		"tracer":  nil,
	}
	b.ReportAllocs()
	for b.Loop() {
		_ = base.RequireAllDependencies(deps)
	}
}
