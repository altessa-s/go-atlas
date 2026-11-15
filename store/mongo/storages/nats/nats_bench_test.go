// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"

	mongohelpers "github.com/altessa-s/go-atlas/store/mongo/internal/testhelpers"
	cursnats "github.com/altessa-s/go-atlas/store/mongo/storages/nats"
)

func BenchmarkStorage_Store(b *testing.B) {
	ns := natstest.StartNATSServer(b)
	_, js := natstest.ConnectJetStream(b, ns)
	kv := natstest.CreateNATSKV(b, js, "bench-cursor", time.Hour)
	ctx := b.Context()
	s, err := cursnats.New(ctx, kv)
	require.NoError(b, err)
	meta := mongohelpers.SampleCursorMetadata()

	for b.Loop() {
		_ = s.Store(ctx, "bench-key", meta)
	}
}

func BenchmarkStorage_Load(b *testing.B) {
	ns := natstest.StartNATSServer(b)
	_, js := natstest.ConnectJetStream(b, ns)
	kv := natstest.CreateNATSKV(b, js, "bench-cursor-load", time.Hour)
	ctx := b.Context()
	s, err := cursnats.New(ctx, kv)
	require.NoError(b, err)
	_ = s.Store(ctx, "bench-key", mongohelpers.SampleCursorMetadata())

	for b.Loop() {
		_, _ = s.Load(ctx, "bench-key")
	}
}
