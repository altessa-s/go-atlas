// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"errors"

	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// GrpcInterPrometheusSamplingStrategy defines how sampling is applied to streaming messages.
type GrpcInterPrometheusSamplingStrategy string

const (
	// GrpcInterPrometheusPerMessageSampling samples individual messages within a stream.
	// Each message is sampled independently based on the sampling rate.
	GrpcInterPrometheusPerMessageSampling GrpcInterPrometheusSamplingStrategy = "per_message"

	// GrpcInterPrometheusPerStreamSampling samples entire streams as a unit.
	// Either the entire stream is sampled or not at all.
	GrpcInterPrometheusPerStreamSampling GrpcInterPrometheusSamplingStrategy = "per_stream"
)

// GrpcInterPrometheusConfig defines the configuration for Prometheus metrics interceptor.
// Controls which metrics are collected and how they are configured.
type GrpcInterPrometheusConfig struct {
	// BaseGrpcInterceptorConfig provides common interceptor configuration.
	BaseGrpcInterceptorConfig `yaml:",inline"`

	// Namespace is the Prometheus metric namespace.
	// Used as the first part of metric names (e.g., "myapp" -> "myapp_grpc_server_requests_total").
	// Empty namespace is allowed and results in metrics without namespace prefix.
	Namespace string `yaml:"namespace" default:""`

	// Subsystem is the Prometheus metric subsystem.
	// Used as the second part of metric names after namespace (e.g., "api" -> "myapp_api_server_requests_total").
	// Default is "grpc" which results in standard gRPC metric names.
	Subsystem string `yaml:"subsystem" default:"grpc"`

	// DurationBuckets defines custom histogram buckets for request duration metrics in seconds.
	// Must be in increasing order. If not specified, reasonable defaults are used.
	// Example: [0.001, 0.01, 0.1, 1.0, 10.0]
	DurationBuckets []float64 `yaml:"durationBuckets"`

	// SizeBuckets defines custom histogram buckets for message size metrics in bytes.
	// Only used when EnableSizeMetrics is true. Must be in increasing order.
	// If not specified, reasonable defaults are used.
	// Example: [1024, 4096, 16384, 65536]
	SizeBuckets []float64 `yaml:"sizeBuckets"`

	// EnableSizeMetrics controls whether request/response size metrics are collected.
	// Size metrics add overhead as they require inspecting message payloads.
	// When enabled, collects grpc_server_request_size_bytes and grpc_server_response_size_bytes histograms.
	EnableSizeMetrics bool `yaml:"enableSizeMetrics" default:"false"`

	// EnableStreamMetrics controls whether per-message streaming metrics are collected.
	// Streaming metrics track individual message counts and sizes within gRPC streams but add overhead.
	// When enabled, collects grpc_server_stream_messages_sent_total and grpc_server_stream_messages_received_total counters.
	EnableStreamMetrics bool `yaml:"enableStreamMetrics" default:"false"`

	// StreamSamplingRate controls the sampling rate for streaming message metrics (0.0 to 1.0).
	// Only effective when EnableStreamMetrics is true. Used to reduce overhead in high-throughput scenarios.
	// 1.0 = no sampling (record all), 0.1 = record 10% of messages, 0.0 = disable streaming metrics.
	StreamSamplingRate float64 `yaml:"streamSamplingRate" default:"1.0"`

	// StreamSamplingStrategy defines how sampling is applied to streaming messages.
	// Only effective when EnableStreamMetrics is true and StreamSamplingRate < 1.0.
	// "per_message" = sample individual messages, "per_stream" = sample entire streams.
	StreamSamplingStrategy GrpcInterPrometheusSamplingStrategy `yaml:"streamSamplingStrategy" default:"per_message"`
}

// IsEnabled returns true if Prometheus metrics collection is enabled.
// This is a convenience method to check if the interceptor should be active.
func (c *GrpcInterPrometheusConfig) IsEnabled() bool {
	return c != nil && c.Enable
}

// Validate performs validation of the Prometheus interceptor configuration.
// Ensures all fields are properly configured for metrics collection.
//
// Validation rules:
//   - DurationBuckets: must be in strictly increasing order if specified
//   - SizeBuckets: must be in strictly increasing order if specified
//   - StreamSamplingRate: must be between 0.0 and 1.0
//   - StreamSamplingStrategy: must be a valid strategy
//   - IgnoreMethods: each method name must be non-empty when specified
//   - IgnorePatterns: each pattern must be non-empty when specified
//
// Returns an error if validation fails, nil otherwise.
func (c *GrpcInterPrometheusConfig) Validate() error {
	return c.ValidateBase(func() error {
		return ValidateStruct(c,
			validation.Field(&c.DurationBuckets, validation.By(validateIncreasingOrder)),
			validation.Field(&c.SizeBuckets, validation.By(validateIncreasingOrder)),
			validation.Field(&c.StreamSamplingRate, validation.Min(0.0), validation.Max(1.0)),
			validation.Field(&c.StreamSamplingStrategy, validation.In(GrpcInterPrometheusPerMessageSampling,
				GrpcInterPrometheusPerStreamSampling)),
		)
	})
}

// DefaultGrpcInterPrometheusConfig returns a GrpcInterPrometheusConfig with default values.
// Prometheus metrics collection is disabled by default.
func DefaultGrpcInterPrometheusConfig() GrpcInterPrometheusConfig {
	return GrpcInterPrometheusConfig{
		BaseGrpcInterceptorConfig: BaseGrpcInterceptorConfig{
			EnableMixin: EnableMixin{Enable: false},
		},
		Subsystem:              "grpc",
		EnableSizeMetrics:      false,
		EnableStreamMetrics:    false,
		StreamSamplingRate:     1.0,
		StreamSamplingStrategy: GrpcInterPrometheusPerMessageSampling,
	}
}

// validateIncreasingOrder checks that float64 slice values are in strictly increasing order.
func validateIncreasingOrder(value any) error {
	buckets, ok := value.([]float64)
	if !ok {
		return nil // Not our type, skip validation
	}

	if len(buckets) == 0 {
		return nil // Empty slice is valid
	}

	for i := range len(buckets) - 1 {
		if buckets[i] >= buckets[i+1] {
			return errors.New("buckets must be in strictly increasing order")
		}
	}

	return nil
}
