// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package observe

//go:generate go tool optgen generate --type=options

import "time"

// DefaultMetricsSubsystem is the Prometheus subsystem name used when no
// explicit subsystem is provided via [WithMetricsSubsystem]. Override it
// when the same process runs several HTTP clients against different
// upstreams so each client's metrics land in their own namespace
// (e.g. egrul_requests_total vs kfocus_requests_total) and do not collide
// when registered against a shared registry.
const DefaultMetricsSubsystem = "http_client"

// Health integration defaults. Override via WithHealthRetryWindow,
// WithHealthRetryThreshold, and WithHealthRetryMinSamples.
const (
	// DefaultHealthServiceName is the service name used when registering
	// the aggregate health checker; mirrors [DefaultMetricsSubsystem]
	// to keep dashboards and alerts visually aligned.
	DefaultHealthServiceName = DefaultMetricsSubsystem
	// DefaultHealthRetryWindow is the rolling window over which retry rate
	// contributes to the aggregate health status.
	DefaultHealthRetryWindow = 60 * time.Second
	// DefaultHealthRetryBucketCount is the number of buckets in the retry-rate ring.
	DefaultHealthRetryBucketCount = 6
	// DefaultHealthRetryDegradedThreshold is the retry-rate fraction (0.0-1.0)
	// above which the checker reports [health.StatusDegraded] regardless
	// of circuit breaker state.
	DefaultHealthRetryDegradedThreshold = 0.2
	// DefaultHealthRetryMinSamples is the minimum number of requests required
	// in the window before retry-rate is allowed to influence the status,
	// preventing flapping on tiny samples.
	DefaultHealthRetryMinSamples = 10
)

// options holds the configuration of an [Observer].
type options struct {
	// metricsSubsystem is the Prometheus subsystem the client metrics are
	// registered under.
	metricsSubsystem string `optgen:"default=DefaultMetricsSubsystem"`
	// healthServiceName is the service name used when registering the aggregate
	// checker. Defaults to [DefaultHealthServiceName] which mirrors
	// [DefaultMetricsSubsystem] for visual consistency in dashboards.
	healthServiceName string `optgen:"default=DefaultHealthServiceName"`
	// WithPerHostHealthChecks enables per-host [health.Checker] registration in
	// addition to the aggregate service. Only hosts that have explicit settings
	// configured via [client.WithCircuitBreakerSettings] are registered — lazy
	// hosts continue to contribute to the aggregate status only. This bound on
	// growth matters because the coordinator keeps registered services alive for
	// the lifetime of the client.
	//
	// Has no effect without a health reporter.
	healthPerHost bool `opt:"PerHostHealthChecks" optgen:"default=false"`
	// healthRetryWindow is the duration of the rolling window over which retry
	// rate contributes to the health status.
	healthRetryWindow time.Duration `optgen:"default=DefaultHealthRetryWindow"`
	// healthRetryBuckets is the number of buckets in the retry-rate ring.
	// More buckets give finer expiry; fewer reduce read-side work.
	healthRetryBuckets int `optgen:"default=DefaultHealthRetryBucketCount"`
	// healthRetryThreshold is the retry-rate fraction (0.0-1.0) above which
	// the aggregate reports [health.StatusDegraded] independently of breaker
	// state.
	healthRetryThreshold float64 `optgen:"default=DefaultHealthRetryDegradedThreshold"`
	// healthRetryMinSamples is the minimum number of requests in the window
	// before retry rate is allowed to lower the status. Below this threshold
	// retry-rate input is ignored to avoid flapping on small samples.
	healthRetryMinSamples uint64 `optgen:"default=DefaultHealthRetryMinSamples"`
}
