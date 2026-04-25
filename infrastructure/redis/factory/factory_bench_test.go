// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"testing"

	"github.com/altessa-s/go-atlas/config"
)

func BenchmarkUniversalOptions(b *testing.B) {
	builder := New(&config.Redis{
		Hosts:    []string{"localhost:6379"},
		Password: "pass",
		PoolSize: 10,
	})
	b.ResetTimer()
	for b.Loop() {
		builder.UniversalOptions()
	}
}

func BenchmarkNew(b *testing.B) {
	cfg := &config.Redis{
		Hosts:    []string{"localhost:6379"},
		Password: "pass",
		PoolSize: 10,
	}
	for b.Loop() {
		New(cfg)
	}
}
