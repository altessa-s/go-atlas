// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/idempotency"
	"github.com/altessa-s/go-atlas/transport/broker/msg"

	memorystorage "github.com/altessa-s/go-atlas/data/idempotency/storages/memory"
	brokeridem "github.com/altessa-s/go-atlas/transport/broker/idempotency"
)

type noopAcker struct{}

func (noopAcker) Ack() error                             { return nil }
func (noopAcker) Nak(_ ...time.Duration) error           { return nil }
func (noopAcker) NakWithBackOff(_ msg.BackOffFunc) error { return nil }
func (noopAcker) Term(_ ...string) error                 { return nil }
func (noopAcker) InProgress() error                      { return nil }

// storageKeySink keeps the built keys escaping, so the benchmark counts the
// allocation a delivery pays.
var storageKeySink string

func BenchmarkBuildStorageKey(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		storageKeySink = brokeridem.BuildStorageKey("bench_consumer", "550e8400-e29b-41d4-a716-446655440000")
	}
}

func BenchmarkWrap_Acquired(b *testing.B) {
	store := memorystorage.New()
	keeper := idempotency.New(store, idempotency.WithMaxLockDuration(time.Minute))

	h := &testSubscriberHandler{
		topic: "topic",
		fn: func(_ context.Context, in *msg.Message) {
			_ = in.Ack()
		},
	}
	wrapped := brokeridem.Wrap(h, keeper, "bench_consumer")
	ctx := b.Context()

	b.ResetTimer()
	b.ReportAllocs()

	var counter int
	for b.Loop() {
		counter++
		m := msg.NewMessageWithMeta("topic", []byte("data"), msg.Meta{
			{Key: msg.MetaKeyDeduplicateId, Value: "key-" + strconv.Itoa(counter)},
		}, msg.WithAcker(noopAcker{}))

		wrapped.Handle(ctx, m)
	}
}

func BenchmarkWrap_Duplicate(b *testing.B) {
	store := memorystorage.New()
	keeper := idempotency.New(store, idempotency.WithMaxLockDuration(time.Minute))

	h := &testSubscriberHandler{
		topic: "topic",
		fn: func(_ context.Context, in *msg.Message) {
			_ = in.Ack()
		},
	}
	wrapped := brokeridem.Wrap(h, keeper, "bench_dup_consumer")
	ctx := b.Context()

	// Seed the key as already completed
	seedMsg := msg.NewMessageWithMeta("topic", []byte("data"), msg.Meta{
		{Key: msg.MetaKeyDeduplicateId, Value: "constant-duplicate-key"},
	}, msg.WithAcker(noopAcker{}))
	wrapped.Handle(ctx, seedMsg)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		m := msg.NewMessageWithMeta("topic", []byte("data"), msg.Meta{
			{Key: msg.MetaKeyDeduplicateId, Value: "constant-duplicate-key"},
		}, msg.WithAcker(noopAcker{}))

		wrapped.Handle(ctx, m)
	}
}
