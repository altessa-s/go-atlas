// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"context"
	"testing"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	locknats "github.com/altessa-s/go-atlas/data/locks/dlock/providers/nats"
)

func benchLocker(b *testing.B) *locknats.Locker {
	b.Helper()
	ns := testhelpers.StartNATSServer(b)
	nc := testhelpers.ConnectNATS(b, ns)
	ctx := b.Context()

	locker, err := locknats.New(ctx, nc, locknats.WithBucket("bench-dlock"))
	if err != nil {
		b.Fatalf("failed to create locker: %v", err)
	}
	b.Cleanup(func() { _ = locker.Close(context.Background()) })
	return locker
}

func BenchmarkLocker_Lock_Release(b *testing.B) {
	locker := benchLocker(b)
	ctx := b.Context()
	for b.Loop() {
		lk, err := locker.Lock(ctx, "bench-key")
		if err != nil {
			b.Fatalf("Lock() error: %v", err)
		}
		_ = lk.Release(ctx)
	}
}

func BenchmarkLocker_GetLockInfo(b *testing.B) {
	locker := benchLocker(b)
	ctx := b.Context()

	lk, _ := locker.Lock(ctx, "bench-info")
	defer lk.Release(ctx) //nolint:errcheck

	for b.Loop() {
		_, _ = locker.GetLockInfo(ctx, "bench-info")
	}
}
