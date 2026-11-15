// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/coord/leadelect/providers"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"

	lenats "github.com/altessa-s/go-atlas/coord/leadelect/providers/nats"
)

func BenchmarkProvider_IsLeader(b *testing.B) {
	ns := natstest.StartNATSServer(b)
	nc := natstest.ConnectNATS(b, ns)
	ctx := b.Context()

	provider, err := lenats.New(ctx, nc, lenats.WithBucket("bench-le"))
	if err != nil {
		b.Fatalf("failed to create provider: %v", err)
	}

	cfg := providers.Config{
		Key:      "bench-election",
		TTL:      5 * time.Second,
		NodeId:   "bench-node",
		LostCh:   make(chan struct{}, 1),
		BecameCh: make(chan struct{}, 1),
		StopCh:   make(chan struct{}, 1),
	}

	_ = provider.Start(ctx, cfg)
	defer provider.Stop(ctx) //nolint:errcheck
	testhelpers.WaitFor(b, 5*time.Second, provider.IsLeader, "provider never became leader")

	for b.Loop() {
		provider.IsLeader()
	}
}

func BenchmarkProvider_IsRunning(b *testing.B) {
	ns := natstest.StartNATSServer(b)
	nc := natstest.ConnectNATS(b, ns)
	ctx := b.Context()

	provider, err := lenats.New(ctx, nc, lenats.WithBucket("bench-le2"))
	if err != nil {
		b.Fatalf("failed to create provider: %v", err)
	}

	for b.Loop() {
		provider.IsRunning()
	}
}
