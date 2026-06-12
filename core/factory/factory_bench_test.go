// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import "testing"

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
