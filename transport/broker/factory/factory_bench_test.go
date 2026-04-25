// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"testing"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/transport/broker"
	"github.com/altessa-s/go-atlas/transport/broker/msg"
)

// benchProvider is a minimal broker.Provider for benchmarks. Build does not call its methods.
type benchProvider struct{}

func (benchProvider) Publish(context.Context, msg.Message) error            { return nil }
func (benchProvider) PublishBatch(context.Context, ...msg.Message) error    { return nil }
func (benchProvider) Subscriber(broker.SubscriberFactory) broker.Subscriber { return nil }

func BenchmarkBrokerBuilder_New(b *testing.B) {
	cfg := &config.Broker{}
	for b.Loop() {
		New(cfg)
	}
}

func BenchmarkBrokerBuilder_Build(b *testing.B) {
	cfg := &config.Broker{}
	provider := benchProvider{}
	builder := New(cfg)
	b.ResetTimer()
	for b.Loop() {
		_, _ = builder.Build(provider)
	}
}
