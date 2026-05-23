// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dispatch

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import (
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas/core/io/wal"
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// Default engine configuration.
const (
	// DefaultBufferSize is the in-memory queue capacity.
	DefaultBufferSize = 10000
	// DefaultBatchSize is the maximum batch size passed to Sink.StoreBatch.
	DefaultBatchSize = 100
	// DefaultFlushInterval is the maximum time before flushing a partial batch.
	DefaultFlushInterval = time.Second
	// DefaultWorkers is the number of dispatch worker goroutines.
	DefaultWorkers = 2
	// DefaultRetryAttempts is the maximum retries per failed batch.
	DefaultRetryAttempts = 3
	// DefaultRetryBackoff is the base duration for exponential backoff.
	DefaultRetryBackoff = 100 * time.Millisecond
	// DefaultMetricsSubsystem is the metrics subsystem name when none is set.
	// Kept as "async" so that dashboards and alerts built against earlier
	// versions of this package (previously located at core/buf/async)
	// continue to match. Override via [WithMetricsSubsystem] when a facade
	// embeds this engine under a domain-specific subsystem.
	DefaultMetricsSubsystem = "async"
)

// defaultLogger returns the slog logger used when no [WithLogger] is supplied.
func defaultLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

// options holds the tunable configuration for [Engine]. Mutate it only
// through the generated [WithXxx] functions or the compound [WithWAL]
// helper below. The struct and all hand-written options are generic in
// T, which is the item type the engine dispatches.
type options[T any] struct {
	bufferSize       int            `optgen:"default=DefaultBufferSize" optval:"positive"`
	batchSize        int            `optgen:"default=DefaultBatchSize"  optval:"positive"`
	flushInterval    time.Duration  `optgen:"default=DefaultFlushInterval"`
	workers          int            `optgen:"default=DefaultWorkers"       optval:"positive"`
	retryAttempts    int            `optgen:"default=DefaultRetryAttempts" optval:"positive=allow_zero"`
	retryBackoff     time.Duration  `optgen:"default=DefaultRetryBackoff"`
	backPressure     bool           //
	onDrop           DropHandler[T] //
	logger           *slog.Logger   `optgen:"default=defaultLogger()"`
	collector        metrics.Collector
	metricsSubsystem string `optgen:"default=DefaultMetricsSubsystem"`

	// WAL fields are mutated only by the hand-written [WithWAL] helper,
	// which sets them together, so optgen must not emit individual
	// setters for them.
	walEnabled bool         `opt:"-"`
	walDir     string       `opt:"-"`
	walOpts    []wal.Option `opt:"-"`
	codec      Codec[T]     `opt:"-"`
}

// WithWAL enables write-ahead logging rooted at dir, using codec to
// serialize items for durability. When WAL is enabled, codec must be
// non-nil; the engine becomes crash-safe and records that have reached
// the most recent fsync are guaranteed to be delivered to the sink
// (at-least-once) even after a process crash. Additional WAL tunables
// (segment size, fsync interval, max bytes) are supplied via the
// variadic walOpts argument — see the [wal] package for the available
// [wal.Option] constructors. Without this option the engine runs purely
// in-memory and incurs no I/O on Submit.
func WithWAL[T any](dir string, codec Codec[T], walOpts ...wal.Option) Option[T] {
	return func(o *options[T]) {
		if codec == nil || dir == "" {
			return
		}
		o.walEnabled = true
		o.walDir = dir
		o.walOpts = walOpts
		o.codec = codec
	}
}
