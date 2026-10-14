// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natsprovider

import (
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas/transport/broker/msg"
)

func BenchmarkIsValidSubject_Valid(b *testing.B) {
	for b.Loop() {
		IsValidSubject("my-topic-123")
	}
}

func BenchmarkIsValidSubject_Invalid(b *testing.B) {
	for b.Loop() {
		IsValidSubject("topic.with.dots")
	}
}

func BenchmarkAppendDeduplicateID(b *testing.B) {
	headers := nats.Header{
		jetstream.MsgIDHeader: {"invoice-9-paid"},
		"tenant":              {"t1"},
	}
	metaData := make([]msg.MetaData, 0, len(headers))
	for b.Loop() {
		metaData = appendDeduplicateID(metaData[:0], headers)
	}
}
