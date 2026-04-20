// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package topics_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/transport/broker/providers/nats/topics"
)

const (
	benchSingle topics.Topic = "events.{TENANT}"
	benchTwo    topics.Topic = "orders.{REGION}.{ORDER_ID}"
)

func BenchmarkTopic_With_Single(b *testing.B) {
	for b.Loop() {
		benchSingle.With("TENANT", "acme")
	}
}

func BenchmarkTopic_With_Multiple(b *testing.B) {
	for b.Loop() {
		benchTwo.With("REGION", "us", "ORDER_ID", "42")
	}
}

func BenchmarkTopic_WithValidation(b *testing.B) {
	for b.Loop() {
		_, _ = benchSingle.WithValidation("TENANT", "acme")
	}
}

func BenchmarkTopic_AcceptsMacros(b *testing.B) {
	keys := []topics.TopicKey{"REGION", "ORDER_ID"}
	for b.Loop() {
		benchTwo.AcceptsMacros(keys...)
	}
}

func BenchmarkTopic_RequiredMacros(b *testing.B) {
	for b.Loop() {
		benchTwo.RequiredMacros()
	}
}

func BenchmarkTopic_Validate(b *testing.B) {
	for b.Loop() {
		_ = benchTwo.Validate()
	}
}
