// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	cursnats "github.com/altessa-s/go-atlas/data/mongo/cursor_storages/nats"
)

func BenchmarkStorage_Store(b *testing.B) {
	ns := testhelpers.StartNATSServer(b)
	_, js := testhelpers.ConnectJetStream(b, ns)
	kv := testhelpers.CreateNATSKV(b, js, "bench-cursor", time.Hour)
	s := cursnats.New(kv)
	ctx := b.Context()
	meta := sampleMetadata()

	for b.Loop() {
		_ = s.Store(ctx, "bench-key", meta)
	}
}

func BenchmarkStorage_Load(b *testing.B) {
	ns := testhelpers.StartNATSServer(b)
	_, js := testhelpers.ConnectJetStream(b, ns)
	kv := testhelpers.CreateNATSKV(b, js, "bench-cursor-load", time.Hour)
	s := cursnats.New(kv)
	ctx := b.Context()
	_ = s.Store(ctx, "bench-key", sampleMetadata())

	for b.Loop() {
		_, _ = s.Load(ctx, "bench-key")
	}
}
