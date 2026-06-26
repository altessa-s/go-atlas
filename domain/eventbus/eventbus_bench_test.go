// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package eventbus_test

import (
	"context"
	"testing"

	"github.com/altessa-s/go-atlas/domain/eventbus"
)

type benchEvent struct{ n int }

func BenchmarkPublish(b *testing.B) {
	bus := eventbus.New()
	eventbus.Subscribe(bus, func(_ context.Context, _ benchEvent) error { return nil })
	ctx := b.Context()
	evt := benchEvent{n: 1}

	b.ReportAllocs()
	for b.Loop() {
		if err := bus.Publish(ctx, evt); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPublishTyped(b *testing.B) {
	bus := eventbus.New()
	eventbus.Subscribe(bus, func(_ context.Context, _ benchEvent) error { return nil })
	ctx := b.Context()
	evt := benchEvent{n: 1}

	b.ReportAllocs()
	for b.Loop() {
		if err := eventbus.Publish(ctx, bus, evt); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPublishNoHandler(b *testing.B) {
	bus := eventbus.New()
	ctx := b.Context()
	evt := benchEvent{n: 1}

	b.ReportAllocs()
	for b.Loop() {
		if err := bus.Publish(ctx, evt); err != nil {
			b.Fatal(err)
		}
	}
}
