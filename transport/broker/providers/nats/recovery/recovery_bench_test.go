// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

import (
	"testing"

	"github.com/nats-io/nats.go/jetstream"
)

func BenchmarkRegistry_HasStream(b *testing.B) {
	r := NewRegistry()
	r.RegisterStream("test", jetstream.StreamConfig{Name: "test"}, RecoveryStrategyAuto)

	for b.Loop() {
		r.HasStream("test")
	}
}

func BenchmarkRegistry_RegisterStream(b *testing.B) {
	r := NewRegistry()
	cfg := jetstream.StreamConfig{Name: "test"}

	for b.Loop() {
		r.RegisterStream("test", cfg, RecoveryStrategyAuto)
	}
}

func BenchmarkRegistry_GetRecoveryStrategy(b *testing.B) {
	r := NewRegistry()
	r.RegisterStream("test", jetstream.StreamConfig{Name: "test"}, RecoveryStrategyAuto)

	for b.Loop() {
		r.GetRecoveryStrategy("test")
	}
}

func BenchmarkRecoveryStrategy_String(b *testing.B) {
	s := RecoveryStrategyAuto
	var str string
	for b.Loop() {
		str = s.String()
	}
	_ = str
}
