// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dlock_test

import (
	"context"
	"testing"

	"github.com/altessa-s/go-atlas/data/locks/dlock"
)

func BenchmarkDLock_Synchronize(b *testing.B) {
	dl := dlock.NewWithNoop()
	ctx := b.Context()
	for b.Loop() {
		_ = dl.Synchronize(ctx, "bench-key", func(ctx context.Context) error {
			return nil
		})
	}
}

func BenchmarkDLock_Lock(b *testing.B) {
	dl := dlock.NewWithNoop()
	ctx := b.Context()
	for b.Loop() {
		lk, _ := dl.Lock(ctx, "bench-key")
		_ = lk.Release(ctx)
	}
}

func BenchmarkDLock_GetLockInfo(b *testing.B) {
	dl := dlock.NewWithNoop()
	ctx := b.Context()
	for b.Loop() {
		_, _ = dl.GetLockInfo(ctx, "bench-key")
	}
}
