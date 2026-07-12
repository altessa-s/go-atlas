// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package defaults

import "testing"

// benchMatched prevents the compiler from optimizing the matching loop away.
var benchMatched bool

// matchAny reports whether method matches any of the default ignore patterns,
// mirroring the per-request check interceptors perform against IgnorePatterns.
func matchAny(method string) bool {
	for _, p := range IgnorePatterns {
		if p.MatchString(method) {
			return true
		}
	}
	return false
}

// BenchmarkIgnorePatterns_TypicalMethod measures the per-request passthrough
// cost of checking a regular RPC method against the default ignore patterns
// (no pattern matches, so both regexes are evaluated).
func BenchmarkIgnorePatterns_TypicalMethod(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		benchMatched = matchAny("/mypackage.MyService/MyMethod")
	}
}

// BenchmarkIgnorePatterns_HealthMethod measures the cost when the method
// matches the health-check ignore pattern.
func BenchmarkIgnorePatterns_HealthMethod(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		benchMatched = matchAny("/grpc.health.v1.Health/Check")
	}
}
