// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package defaults

import "testing"

// matchAny mirrors the per-request check every middleware performs against
// IgnorePatterns before processing a request.
func matchAny(path string) bool {
	for _, p := range IgnorePatterns {
		if p.MatchString(path) {
			return true
		}
	}
	return false
}

func BenchmarkIgnorePatterns_NoMatch(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		matchAny("/api/v1/users/12345")
	}
}

func BenchmarkIgnorePatterns_MatchHealth(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		matchAny("/healthz")
	}
}
