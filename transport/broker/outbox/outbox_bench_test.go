// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/transport/broker/msg"
	"github.com/altessa-s/go-atlas/transport/broker/outbox"
)

// nopStore is a no-op outbox.Store. The publish path never touches the broker;
// with a no-op store the benchmarks isolate the in-process cost of Publish:
// message-to-event conversion (JSON envelope marshal, created-time parsing)
// plus Save normalization (UUID assignment, status defaults, metrics).
type nopStore struct{}

func (nopStore) FetchUnprocessedEvents(context.Context, uint32) ([]outbox.Event, error) {
	return nil, nil
}
func (nopStore) DeleteProcessedEvents(context.Context, time.Duration) error { return nil }
func (nopStore) UnlockStuckEvents(context.Context, time.Duration) error     { return nil }
func (nopStore) SaveEvents(context.Context, ...outbox.Event) error          { return nil }
func (nopStore) UpdateEvents(context.Context, ...outbox.Event) error        { return nil }
func (nopStore) ExpireEvents(context.Context) (int64, error)                { return 0, nil }
func (nopStore) Stats(context.Context) (outbox.Stats, error)                { return outbox.Stats{}, nil }

// nopPublisher satisfies outbox.Publisher; Publish/PublishBatch never call it.
type nopPublisher struct{}

func (nopPublisher) Publish(context.Context, msg.Message) error { return nil }

func BenchmarkOutbox_Publish(b *testing.B) {
	ob := outbox.New(nopStore{}, nopPublisher{})
	m := msg.NewMessage("bench.topic", []byte(`{"id":"42","name":"bench"}`))
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		if err := ob.Publish(ctx, *m); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkOutbox_PublishBatch(b *testing.B) {
	ob := outbox.New(nopStore{}, nopPublisher{})
	msgs := make([]msg.Message, 16)
	for i := range msgs {
		msgs[i] = *msg.NewMessage("bench.topic", []byte(`{"id":"42","name":"bench"}`))
	}
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		if err := ob.PublishBatch(ctx, msgs...); err != nil {
			b.Fatal(err)
		}
	}
}
