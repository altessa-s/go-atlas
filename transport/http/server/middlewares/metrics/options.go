// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate --type=options

import (
	"log/slog"
	"regexp"

	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/defaults"

	prominternal "github.com/altessa-s/go-atlas/transport/internal/prometheus"
)

// Use defaults package for optgen code generation
var _ = defaults.IgnorePatterns

// DefaultMetricsSubsystem is the Prometheus subsystem name used when no
// explicit subsystem is provided via [WithMetricsSubsystem]. Override it
// when several HTTP servers run in the same process so each server's
// metrics land in their own namespace and do not collide when registered
// against a shared [metrics.Collector].
const DefaultMetricsSubsystem = "http"

// Default bucket configurations (use shared defaults from internal package)
var (
	// DefaultDurationBuckets provides reasonable latency buckets for HTTP requests.
	DefaultDurationBuckets = prominternal.DefaultDurationBuckets

	// DefaultSizeBuckets provides reasonable message size buckets for HTTP requests.
	DefaultSizeBuckets = prominternal.DefaultSizeBuckets
)

// options holds configuration for the Prometheus middleware.
type options struct {
	collector         metrics.Collector `optgen:"notnil"`
	metricsSubsystem  string            `optgen:"default=DefaultMetricsSubsystem"`
	logger            *slog.Logger
	durationBuckets   []float64 `opt:"-"`
	sizeBuckets       []float64 `opt:"-"`
	enableSizeMetrics bool
	ignorePaths       []string
	ignorePatterns    []*regexp.Regexp `optgen:"default=defaults.IgnorePatterns"`
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
// Only effective when used with WithEnableSizeMetrics(true).
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
