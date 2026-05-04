// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package loader

import "testing"

// White-box benches for the env-value expansion hot path. The function is
// unexported and runs once per env-key during config loading, so direct
// benchmarking gives a tighter signal than going through the full loader.

func BenchmarkUnwrapEnvValue_Plain(b *testing.B) {
	for b.Loop() {
		_ = unwrapEnvValue("plain string with no dollar")
	}
}

func BenchmarkUnwrapEnvValue_Mixed(b *testing.B) {
	b.Setenv("BENCH_VAR", "expanded")
	for b.Loop() {
		_ = unwrapEnvValue("prefix-$BENCH_VAR-suffix")
	}
}

func BenchmarkUnwrapEnvValue_Escape(b *testing.B) {
	for b.Loop() {
		_ = unwrapEnvValue("regex-pattern-$$end")
	}
}

func BenchmarkUnwrapEnvValue_TrailingDollar(b *testing.B) {
	for b.Loop() {
		_ = unwrapEnvValue(`.*/(get.*|list.*|exists)$`)
	}
}

func BenchmarkUnwrapEnvValueStrict_Plain(b *testing.B) {
	for b.Loop() {
		_, _ = unwrapEnvValueStrict("plain string with no dollar")
	}
}

func BenchmarkUnwrapEnvValueStrict_Mixed(b *testing.B) {
	b.Setenv("BENCH_VAR", "expanded")
	for b.Loop() {
		_, _ = unwrapEnvValueStrict("prefix-$BENCH_VAR-suffix")
	}
}
