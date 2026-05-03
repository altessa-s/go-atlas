// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate --type=options

import (
	"log/slog"
	"regexp"

	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/defaults"

	prominternal "github.com/altessa-s/go-atlas/transport/internal/prometheus"
)

// Use defaults package for optgen code generation
var _ = defaults.IgnorePatterns

// Metric labels (use shared interned labels).
var (
	methodLabel    = prominternal.MethodLabel
	statusLabel    = prominternal.StatusLabel
	directionLabel = prominternal.DirectionLabel
)

// Direction label values for streaming metrics (use shared interned values)
var (
	directionSent     = prominternal.DirectionSent
	directionReceived = prominternal.DirectionReceived
)

// Sampling strategy constants
const (
	DefaultStreamSamplingRate = 1.0 // No sampling by default
	MaxStreamSamplingRate     = 1.0
	MinStreamSamplingRate     = 0.0
)

// StreamSamplingStrategy defines how sampling is applied to streaming messages
type StreamSamplingStrategy string

const (
	// PerMessageSampling samples individual messages within a stream
	// Each message is sampled independently based on the sampling rate
	PerMessageSampling StreamSamplingStrategy = "per_message"

	// PerStreamSampling samples entire streams as a unit
	// Either the entire stream is sampled or not at all
	PerStreamSampling StreamSamplingStrategy = "per_stream"
)

// DefaultMetricsSubsystem is the Prometheus subsystem name used when no
// explicit subsystem is provided via [WithMetricsSubsystem]. Override it
// when several gRPC servers run in the same process so each server's
// metrics land in their own namespace and do not collide when registered
// against a shared [metrics.Collector].
const DefaultMetricsSubsystem = "grpc"

// Default bucket configurations (use shared defaults from internal package)
var (
	// DefaultDurationBuckets provides reasonable latency buckets for gRPC requests.
	DefaultDurationBuckets = prominternal.DefaultDurationBuckets

	// DefaultSizeBuckets provides reasonable message size buckets for gRPC requests.
	DefaultSizeBuckets = prominternal.DefaultSizeBuckets
)

type options struct {
	collector              metrics.Collector `optgen:"notnil"`
	metricsSubsystem       string            `optgen:"default=DefaultMetricsSubsystem"`
	logger                 *slog.Logger
	durationBuckets        []float64 `opt:"-" optgen:"default=DefaultDurationBuckets"`
	sizeBuckets            []float64 `opt:"-" optgen:"default=DefaultSizeBuckets"`
	enableSizeMetrics      bool
	enableStreamMetrics    bool
	ignoreMethods          []string
	ignorePatterns         []*regexp.Regexp       `optgen:"default=defaults.IgnorePatterns"`
	streamSamplingRate     float64                `optgen:"default=DefaultStreamSamplingRate"`
	streamSamplingStrategy StreamSamplingStrategy `optgen:"default=PerMessageSampling"`
}

// WithDurationBuckets returns an option that sets custom histogram buckets for request duration metrics.
// The buckets should be specified in seconds and in increasing order.
//
// Example: WithDurationBuckets([]float64{0.001, 0.01, 0.1, 1.0, 10.0})
func WithDurationBuckets(buckets []float64) Option {
	return func(o *options) {
		if len(buckets) == 0 {
			return
		}

		// Validate buckets are in increasing order
		prominternal.MustValidateBuckets(buckets, "duration")
		o.durationBuckets = prominternal.CopyBuckets(buckets)
	}
}

// WithSizeBuckets returns an option that sets custom histogram buckets for message size metrics.
// The buckets should be specified in bytes and in increasing order.
// Only effective when used with WithEnableSizeMetrics().
//
// Example: WithSizeBuckets([]float64{1024, 4096, 16384, 65536})
func WithSizeBuckets(buckets []float64) Option {
	return func(o *options) {
		if len(buckets) == 0 {
			return
		}

		// Validate buckets are in increasing order
		prominternal.MustValidateBuckets(buckets, "size")
		o.sizeBuckets = prominternal.CopyBuckets(buckets)
	}
}
