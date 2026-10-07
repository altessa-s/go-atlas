// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"testing"
	"time"

	natsconfig "github.com/altessa-s/go-atlas/config/nats"
)

func BenchmarkNatsOptions(b *testing.B) {
	builder := New(&natsconfig.Config{
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
	cfg := &natsconfig.Consumer{
		Description:    "bench",
		DurableName:    "bench-durable",
		FilterSubjects: []string{"events.>"},
		MaxAckPending:  100,
		AckWait:        30 * time.Second,
		DeliverPolicy:  natsconfig.DeliverPolicyAll,
		AckPolicy:      natsconfig.AckPolicyExplicit,
	}
	b.ResetTimer()
	for b.Loop() {
		ConsumerConfig(cfg)
	}
}

func BenchmarkNew(b *testing.B) {
	cfg := &natsconfig.Config{
		Hosts:      []string{"nats://localhost:4222"},
		ClientName: "bench",
	}
	for b.Loop() {
		New(cfg)
	}
}
