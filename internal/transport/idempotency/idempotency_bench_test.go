// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/internal/transport/idempotency"
)

func BenchmarkDefaultKeyValidator(b *testing.B) {
	for b.Loop() {
		_ = idempotency.DefaultKeyValidator("550e8400-e29b-41d4-a716-446655440000")
	}
}

// outcomeSink keeps ClassifyLock results live.
var outcomeSink idempotency.LockOutcome

func BenchmarkClassifyLock(b *testing.B) {
	b.Run("acquired", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			outcomeSink, _ = idempotency.ClassifyLock(true, true, idempotency.StatusInProgress, nil, idempotency.NoLockError)
		}
	})
	b.Run("completed", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			outcomeSink, _ = idempotency.ClassifyLock(false, true, idempotency.StatusSuccess, "entity-1", idempotency.NoLockError)
		}
	})
}
