// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"fmt"
	"testing"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	idempnats "github.com/altessa-s/go-atlas/data/idempotency/storages/nats"
)

func benchSetup(b *testing.B) *idempnats.Storage {
	b.Helper()
	ns := testhelpers.StartNATSServer(b)
	_, js := testhelpers.ConnectJetStream(b, ns)

	storage, err := idempnats.New(js, idempnats.WithBucket(fmt.Sprintf("bench-%d", b.N)))
	if err != nil {
		b.Fatalf("failed to create storage: %v", err)
	}
	return storage
}

func BenchmarkStorage_AttemptLock(b *testing.B) {
	storage := benchSetup(b)
	ctx := b.Context()
	for b.Loop() {
		_, _, _ = storage.AttemptLock(ctx, "bench-key", []byte("val"))
	}
}

func BenchmarkStorage_Complete(b *testing.B) {
	storage := benchSetup(b)
	ctx := b.Context()
	_, _, _ = storage.AttemptLock(ctx, "bench-key", []byte("val"))

	for b.Loop() {
		_ = storage.Complete(ctx, "bench-key", []byte("complete"))
	}
}
