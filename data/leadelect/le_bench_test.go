// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package leadelect_test

import (
	"context"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/leadelect"
	"github.com/altessa-s/go-atlas/data/leadelect/providers"
)

func BenchmarkLeader_IsLeader(b *testing.B) {
	prov := &mockProvider{isLeader: true}
	le := leadelect.New(prov, leadelect.Config{})

	for b.Loop() {
		le.IsLeader()
	}
}

func BenchmarkLeader_RegisterOnLeaderLost(b *testing.B) {
	prov := &mockProvider{}
	le := leadelect.New(prov, leadelect.Config{})
	cb := func(_ context.Context, _ leadelect.LeaderElector) {}

	for b.Loop() {
		le.RegisterOnLeaderLost(cb)
	}
}

func BenchmarkLeader_StartStop(b *testing.B) {
	prov := &mockProvider{}
	cfg := leadelect.Config{Key: "bench", TTL: time.Second, NodeId: "n1"}
	ctx := b.Context()
	_ = providers.Config{} // ensure import

	for b.Loop() {
		le := leadelect.New(prov, cfg)
		_ = le.Start(ctx)
		_ = le.Stop(ctx)
	}
}
