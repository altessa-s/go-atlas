// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package jetstream

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import (
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas/core/encoding/serializer"
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// Default values for the engine options.
const (
	// DefaultStream is the JetStream stream that carries start commands. One
	// stream can serve several saga definitions; each consumes its own subject.
	DefaultStream = "SAGA_COMMANDS"
	// DefaultSubjectPrefix prefixes the per-definition command subject:
	// <prefix>.<definition name>.
	DefaultSubjectPrefix = "saga.start"
	// DefaultConcurrency is the maximum number of sagas one engine drives at once.
	DefaultConcurrency = 16
	// DefaultAckWait is the consumer acknowledgement deadline used when the
	// engine creates the durable consumer. Running executions heartbeat at a
	// third of the consumer's actual AckWait.
	DefaultAckWait = 30 * time.Second
	// DefaultMaxDeliver is the delivery cap used when the engine creates the
	// durable consumer; -1 means unlimited.
	DefaultMaxDeliver = -1
	// DefaultDuplicateWindow is the stream de-duplication window used when the
	// engine creates the stream.
	DefaultDuplicateWindow = 2 * time.Minute
	// DefaultRetryBaseDelay is the first redelivery delay after an interrupted
	// execution.
	DefaultRetryBaseDelay = time.Second
	// DefaultRetryMaxDelay caps the redelivery delay.
	DefaultRetryMaxDelay = time.Minute
)

// options holds the engine configuration. Stream and consumer settings
// (maxAge, duplicateWindow, ackWait, maxDeliver) apply only when the engine
// creates the resource; an existing stream or consumer is never modified.
type options struct {
	stream          string        `optgen:"default=DefaultStream"`
	subjectPrefix   string        `optgen:"default=DefaultSubjectPrefix"`
	consumer        string        // empty → "saga-" + definition name
	concurrency     int           `optgen:"default=DefaultConcurrency" optval:"positive"`
	ackWait         time.Duration `optgen:"default=DefaultAckWait" optval:"positive"`
	maxDeliver      int           `optgen:"default=DefaultMaxDeliver"`
	duplicateWindow time.Duration `optgen:"default=DefaultDuplicateWindow" optval:"positive"`
	maxAge          time.Duration // 0 → unlimited
	retryBaseDelay  time.Duration `optgen:"default=DefaultRetryBaseDelay" optval:"positive"`
	retryMaxDelay   time.Duration `optgen:"default=DefaultRetryMaxDelay" optval:"positive"`

	serializer serializer.Serializer `optgen:"notnil"`
	logger     *slog.Logger
	collector  metrics.Collector `optgen:"notnil"`
}
