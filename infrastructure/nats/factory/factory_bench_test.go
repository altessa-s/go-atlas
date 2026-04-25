// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/config"
)

func BenchmarkNatsOptions(b *testing.B) {
	builder := New(&config.Nats{
		Hosts:          []string{"nats://localhost:4222"},
		ClientName:     "bench",
		ConnectTimeout: 5 * time.Second,
		ReconnectWait:  2 * time.Second,
		PingInterval:   time.Minute,
		MaxPingsOut:    3,
	})
	b.ResetTimer()
	for b.Loop() {
		builder.NatsOptions()
	}
}

func BenchmarkConsumerConfig(b *testing.B) {
	cfg := &config.NatsConsumer{
		Description:    "bench",
		DurableName:    "bench-durable",
		FilterSubjects: []string{"events.>"},
		MaxAckPending:  100,
		AckWait:        30 * time.Second,
		DeliverPolicy:  config.DeliverPolicyAll,
		AckPolicy:      config.AckPolicyExplicit,
	}
	b.ResetTimer()
	for b.Loop() {
		ConsumerConfig(cfg)
	}
}

func BenchmarkNew(b *testing.B) {
	cfg := &config.Nats{
		Hosts:      []string{"nats://localhost:4222"},
		ClientName: "bench",
	}
	for b.Loop() {
		New(cfg)
	}
}
