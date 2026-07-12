// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package broker_test

import (
	"context"
	"testing"

	"github.com/altessa-s/go-atlas/transport/broker"
	"github.com/altessa-s/go-atlas/transport/broker/msg"
)

// noopProvider discards all messages so benchmarks measure the Broker
// publish path (metrics, outbox indirection) rather than transport cost.
type noopProvider struct{}

func (noopProvider) Publish(context.Context, msg.Message) error         { return nil }
func (noopProvider) PublishBatch(context.Context, ...msg.Message) error { return nil }
func (noopProvider) Subscriber(f broker.SubscriberFactory) broker.Subscriber {
	return f(noopProvider{})
}

func BenchmarkPublish(b *testing.B) {
	br := broker.New(noopProvider{})
	ctx := b.Context()
	m := msg.Message{Topic: "orders.created", Data: []byte(`{"id":1}`)}

	b.ReportAllocs()
	for b.Loop() {
		_ = br.Publish(ctx, m)
	}
}

func BenchmarkPublishBatch(b *testing.B) {
	br := broker.New(noopProvider{})
	ctx := b.Context()
	msgs := []msg.Message{
		{Topic: "orders.created", Data: []byte(`{"id":1}`)},
		{Topic: "orders.updated", Data: []byte(`{"id":2}`)},
		{Topic: "orders.deleted", Data: []byte(`{"id":3}`)},
	}

	b.ReportAllocs()
	for b.Loop() {
		_ = br.PublishBatch(ctx, msgs...)
	}
}
