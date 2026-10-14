// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	limitnats "github.com/altessa-s/go-atlas/data/limiters/storages/nats"
)

func benchSetup(b *testing.B) *limitnats.Provider {
	b.Helper()
	ns := testhelpers.StartNATSServer(b)
	_, js := testhelpers.ConnectJetStream(b, ns)

	provider, err := limitnats.New(js, limitnats.WithBucket(fmt.Sprintf("bench-%d", b.N)))
	if err != nil {
		b.Fatalf("failed to create provider: %v", err)
	}
	return provider
}

func BenchmarkProvider_Allow(b *testing.B) {
	provider := benchSetup(b)
	ctx := b.Context()
	for b.Loop() {
		_, _ = provider.Allow(ctx, "bench-key", 1000000, time.Minute)
	}
}

// BenchmarkProvider_Allow_Contended races parallel callers on one key. The
// limit is never reached and the window is short, so every call admits and
// writes — each one a CAS that can lose to the others — while the stored
// window stays small.
func BenchmarkProvider_Allow_Contended(b *testing.B) {
	provider := benchSetup(b)
	ctx := b.Context()
	b.SetParallelism(4)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := provider.Allow(ctx, "contended-key", 1<<40, 50*time.Millisecond); err != nil {
				b.Error(err)
				return
			}
		}
	})
}

func BenchmarkProvider_Reset(b *testing.B) {
	provider := benchSetup(b)
	ctx := b.Context()
	for b.Loop() {
		_ = provider.Reset(ctx, "bench-key")
	}
}
