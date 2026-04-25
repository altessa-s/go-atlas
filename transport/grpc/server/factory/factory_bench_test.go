// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"testing"

	"github.com/altessa-s/go-atlas/config"
)

func BenchmarkServerBuilder_New(b *testing.B) {
	cfg := &config.Grpc{ListenAddress: "0.0.0.0:0"}
	for b.Loop() {
		New(cfg)
	}
}

func BenchmarkServerBuilder_Build(b *testing.B) {
	cfg := &config.Grpc{ListenAddress: "0.0.0.0:0"}
	builder := New(cfg)
	b.ResetTimer()
	for b.Loop() {
		_, _ = builder.Build()
	}
}
