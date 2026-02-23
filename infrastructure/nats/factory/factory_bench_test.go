// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/config"
)

func BenchmarkNatsOptionsFromConfig(b *testing.B) {
	f := New()
	cfg := &config.Nats{
		Hosts:          []string{"nats://localhost:4222"},
		ClientName:     "bench",
		ConnectTimeout: 5 * time.Second,
		ReconnectWait:  2 * time.Second,
		PingInterval:   time.Minute,
		MaxPingsOut:    3,
	}
	b.ResetTimer()
	for b.Loop() {
		f.NatsOptionsFromConfig(cfg)
	}
}

func BenchmarkConsumerConfigFromConfig(b *testing.B) {
	f := New()
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
		f.ConsumerConfigFromConfig(cfg)
	}
}

func BenchmarkNew(b *testing.B) {
	for b.Loop() {
		New()
	}
}
