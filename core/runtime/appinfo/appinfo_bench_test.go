// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package appinfo_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/core/runtime/appinfo"
)

func BenchmarkEnv(b *testing.B) {
	b.Setenv("BENCH_APPINFO_ENV", "value")
	var sink string
	for b.Loop() {
		sink = appinfo.Env("BENCH_APPINFO_ENV")
	}
	_ = sink
}

func BenchmarkEnvOrHit(b *testing.B) {
	b.Setenv("BENCH_APPINFO_ENVOR", "value")
	var sink string
	for b.Loop() {
		sink = appinfo.EnvOr("BENCH_APPINFO_ENVOR", "fallback")
	}
	_ = sink
}

func BenchmarkEnvOrMiss(b *testing.B) {
	var sink string
	for b.Loop() {
		sink = appinfo.EnvOr("BENCH_APPINFO_MISSING_NEVER_SET", "fallback")
	}
	_ = sink
}

func BenchmarkEnvCachedHit(b *testing.B) {
	b.Setenv("BENCH_APPINFO_CACHED", "cached")
	appinfo.ClearEnvCache()
	_ = appinfo.EnvCached("BENCH_APPINFO_CACHED")
	var sink string
	for b.Loop() {
		sink = appinfo.EnvCached("BENCH_APPINFO_CACHED")
	}
	_ = sink
}

func BenchmarkHomeDir(b *testing.B) {
	var sink string
	for b.Loop() {
		sink = appinfo.HomeDir()
	}
	_ = sink
}

func BenchmarkExpandPathTilde(b *testing.B) {
	var sink string
	for b.Loop() {
		sink = appinfo.ExpandPath("~/Documents")
	}
	_ = sink
}

func BenchmarkExpandPathAbsolute(b *testing.B) {
	var sink string
	for b.Loop() {
		sink = appinfo.ExpandPath("/tmp/file")
	}
	_ = sink
}
