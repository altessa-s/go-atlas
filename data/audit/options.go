// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit

import (
	"log/slog"
	"time"
)

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate --type=options

// Default dispatcher configuration.
const (
	// DefaultBufferSize is the event channel buffer capacity.
	DefaultBufferSize = 10000
	// DefaultBatchSize is the number of events per storage write.
	DefaultBatchSize = 100
	// DefaultFlushInterval is the maximum wait before flushing a partial batch.
	DefaultFlushInterval = time.Second
	// DefaultWorkers is the number of concurrent dispatch goroutines.
	DefaultWorkers = 2
	// DefaultRetryAttempts is the maximum retries per failed batch.
	DefaultRetryAttempts = 3
	// DefaultRetryBackoff is the base duration for exponential backoff.
	DefaultRetryBackoff = 100 * time.Millisecond
	// DefaultShutdownTimeout is the maximum time to wait during graceful shutdown.
	DefaultShutdownTimeout = 30 * time.Second
)

// DropHandler is called when an event is dropped due to a full buffer.
type DropHandler func(event *Event)

type options struct {
	serviceInfo     ServiceInfo
	bufferSize      int           `optgen:"default=DefaultBufferSize"`
	batchSize       int           `optgen:"default=DefaultBatchSize"`
	flushInterval   time.Duration `optval:"positive" optgen:"default=DefaultFlushInterval"`
	workers         int           `optgen:"default=DefaultWorkers"`
	retryAttempts   int           `optgen:"default=DefaultRetryAttempts"`
	retryBackoff    time.Duration `optval:"positive" optgen:"default=DefaultRetryBackoff"`
	shutdownTimeout time.Duration `optval:"positive" optgen:"default=DefaultShutdownTimeout"`
	logger          *slog.Logger
	onDrop          DropHandler
	backPressure    bool
}
