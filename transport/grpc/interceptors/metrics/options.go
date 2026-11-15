// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

//go:generate go tool optgen generate --type=options

import (
	"log/slog"
	"regexp"
	"slices"

	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/defaults"
)

// Use defaults package for optgen code generation
var _ = defaults.IgnorePatterns

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
// against a shared [metrics.Meter].
const DefaultMetricsSubsystem = "grpc"

// DefaultMaxMethods is the default number of distinct method label values the
// interceptor tracks (see [WithMaxMethods]).
const DefaultMaxMethods = 1024

// OverflowMethod is the method label value that collects every method seen
// after the [WithMaxMethods] limit is reached.
const OverflowMethod = "other"

type options struct {
	meter            metrics.Meter `optgen:"default=metrics.Noop(),notnil"`
	metricsSubsystem string        `optgen:"default=DefaultMetricsSubsystem"`
	logger           *slog.Logger
	// Set by WithDurationBuckets: validates ordering and copies the caller's slice.
	durationBuckets []float64 `optgen:"manual,default=DefaultDurationBuckets"`
	// Set by WithSizeBuckets: validates ordering and copies the caller's slice.
	sizeBuckets            []float64 `optgen:"manual,default=DefaultSizeBuckets"`
	enableSizeMetrics      bool
	enableStreamMetrics    bool
	ignoreMethods          []string
	ignorePatterns         []*regexp.Regexp       `optgen:"default=defaults.IgnorePatterns"`
	streamSamplingRate     float64                `optgen:"default=DefaultStreamSamplingRate"`
	streamSamplingStrategy StreamSamplingStrategy `optgen:"default=PerMessageSampling"`
	// maxMethods caps the distinct method label values. Method names are
	// client-controlled when the server installs grpc.UnknownServiceHandler,
	// so methods seen after the cap is reached are recorded under
	// [OverflowMethod] instead of minting new series.
	maxMethods int `optgen:"default=DefaultMaxMethods" optval:"positive"`
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
		metrics.MustValidateBuckets(buckets, "duration")
		o.durationBuckets = slices.Clone(buckets)
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
		metrics.MustValidateBuckets(buckets, "size")
		o.sizeBuckets = slices.Clone(buckets)
	}
}
