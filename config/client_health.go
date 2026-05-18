// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"time"

	"github.com/altessa-s/go-atlas/core/collections/slices"

	grpcclient "github.com/altessa-s/go-atlas/transport/grpc/client"
	httpclient "github.com/altessa-s/go-atlas/transport/http/client"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for HealthClient configuration.
const (
	// DefaultHealthClientRetryWindow is the default sliding window for retry rate calculation.
	DefaultHealthClientRetryWindow = 60 * time.Second

	// DefaultHealthClientRetryThreshold is the default retry rate threshold for degradation.
	DefaultHealthClientRetryThreshold = 0.2

	// DefaultHealthClientRetryMinSamples is the default minimum samples before retry rate affects health.
	DefaultHealthClientRetryMinSamples = uint64(10)

	// DefaultHealthClientRetryBuckets is the default number of buckets in the sliding window.
	DefaultHealthClientRetryBuckets = 60
)

// HealthClientStateMapper defines the mapping strategy from gRPC connection states to health status.
type HealthClientStateMapper string

const (
	// HealthClientStateMapperDefault uses the default mapping from connectivity.State to ServingStatus.
	HealthClientStateMapperDefault HealthClientStateMapper = "default"

	// HealthClientStateMapperStrict maps any non-READY state to NOT_SERVING.
	HealthClientStateMapperStrict HealthClientStateMapper = "strict"

	// HealthClientStateMapperLenient maps CONNECTING/IDLE to SERVING, only TRANSIENT_FAILURE/SHUTDOWN to NOT_SERVING.
	HealthClientStateMapperLenient HealthClientStateMapper = "lenient"
)

// HealthClient contains base health monitoring configuration for external service clients.
// This is embedded in transport-specific health configurations.
type HealthClient struct {
	// ServiceName is the name registered with the health coordinator.
	// This name appears in health status reports and metrics.
	// Required field.
	ServiceName string `yaml:"serviceName"`
}

// HTTPHealthClient contains health monitoring configuration for HTTP clients.
// Health is derived from retry rate and circuit breaker state.
//
// Example:
//
//	health := &config.HTTPHealthClient{
//	    HealthClient: config.HealthClient{
//	        ServiceName: "openai-api",
//	    },
//	    RetryWindow:     60 * time.Second,
//	    RetryThreshold:  0.15,
//	    RetryMinSamples: 5,
//	}
//
//	httpClient := httpclient.New(append(
//	    []httpclient.Option{httpclient.WithHealthCoordinator(coord)},
//	    health.Options()...,
//	)...)
type HTTPHealthClient struct {
	HealthClient `yaml:",inline"`

	// RetryWindow is the sliding window duration for calculating retry rates.
	// Health status degrades when retry rate exceeds threshold within this window.
	// Defaults to 60s.
	RetryWindow time.Duration `yaml:"retryWindow" default:"60s"`

	// RetryThreshold is the maximum acceptable retry rate (0.0-1.0).
	// When exceeded, the service health status changes to DEGRADED.
	// Defaults to 0.2 (20% retry rate).
	RetryThreshold float64 `yaml:"retryThreshold" default:"0.2"`

	// RetryMinSamples is the minimum number of requests in the window
	// before retry rate affects health status.
	// Defaults to 10.
	RetryMinSamples uint64 `yaml:"retryMinSamples" default:"10"`

	// RetryBuckets is the number of buckets in the sliding window.
	// More buckets provide finer granularity at the cost of memory.
	// Defaults to 60.
	RetryBuckets int `yaml:"retryBuckets" default:"60"`

	// PerHost enables per-host health checks.
	// When true, registers separate health checkers for each configured host.
	// Only applies to clients with explicit per-host circuit breaker settings.
	PerHost bool `yaml:"perHost" default:"false"`
}

// GRPCHealthClient contains health monitoring configuration for gRPC clients.
// Health is derived from connection state.
//
// Example:
//
//	health := &config.GRPCHealthClient{
//	    HealthClient: config.HealthClient{
//	        ServiceName: "user-service",
//	    },
//	    StateMapper: config.HealthClientStateMapperStrict,
//	}
//
//	grpcClient, _ := grpcclient.New(ctx, target, append(
//	    []grpcclient.Option{grpcclient.WithHealthCoordinator(coord)},
//	    health.Options()...,
//	)...)
type GRPCHealthClient struct {
	HealthClient `yaml:",inline"`

	// StateMapper selects the mapping strategy from gRPC connection state to health status.
	// Defaults to "default".
	StateMapper HealthClientStateMapper `yaml:"stateMapper" default:"default"`

	// PerTarget enables per-target health checks for connection pools.
	// When true, registers separate health checkers for each target address.
	PerTarget bool `yaml:"perTarget" default:"false"`
}

// DefaultHealthClient returns a HealthClient configuration with default values.
// Note: ServiceName must be set explicitly as it's a required field.
func DefaultHealthClient() HealthClient {
	return HealthClient{}
}

// DefaultHTTPHealthClient returns an HTTPHealthClient configuration with default values.
// Note: ServiceName must be set explicitly as it's a required field.
func DefaultHTTPHealthClient() HTTPHealthClient {
	return HTTPHealthClient{
		RetryWindow:     DefaultHealthClientRetryWindow,
		RetryThreshold:  DefaultHealthClientRetryThreshold,
		RetryMinSamples: DefaultHealthClientRetryMinSamples,
		RetryBuckets:    DefaultHealthClientRetryBuckets,
	}
}

// DefaultGRPCHealthClient returns a GRPCHealthClient configuration with default values.
// Note: ServiceName must be set explicitly as it's a required field.
func DefaultGRPCHealthClient() GRPCHealthClient {
	return GRPCHealthClient{
		StateMapper: HealthClientStateMapperDefault,
	}
}

// Validate performs validation of the HealthClient configuration.
// Returns an error if validation fails, nil otherwise.
func (h *HealthClient) Validate() error {
	return ValidateStruct(h,
		validation.Field(&h.ServiceName,
			validation.Required),
	)
}

// Validate performs validation of the HTTPHealthClient configuration.
// Returns an error if validation fails, nil otherwise.
func (h *HTTPHealthClient) Validate() error {
	if err := h.HealthClient.Validate(); err != nil {
		return err
	}
	return ValidateStruct(h,
		validation.Field(&h.RetryWindow,
			validation.When(h.RetryWindow > 0, validation.Min(time.Second))),
		validation.Field(&h.RetryThreshold,
			validation.Min(0.0),
			validation.Max(1.0)),
		validation.Field(&h.RetryMinSamples,
			validation.When(h.RetryMinSamples > 0, validation.Min(uint64(1)))),
		validation.Field(&h.RetryBuckets,
			validation.When(h.RetryBuckets > 0, validation.Min(1), validation.Max(3600))),
	)
}

// Validate performs validation of the GRPCHealthClient configuration.
// Returns an error if validation fails, nil otherwise.
func (h *GRPCHealthClient) Validate() error {
	if err := h.HealthClient.Validate(); err != nil {
		return err
	}
	return ValidateStruct(h,
		validation.Field(&h.StateMapper,
			validation.In(
				HealthClientStateMapper(""),
				HealthClientStateMapperDefault,
				HealthClientStateMapperStrict,
				HealthClientStateMapperLenient,
			)),
	)
}

// Options returns HTTP client options for health monitoring.
// The returned options should be used together with WithHealthCoordinator.
//
// Returns nil if h is nil, allowing optional health configuration.
//
// Example:
//
//	client := httpclient.New(append(
//	    []httpclient.Option{
//	        httpclient.WithHealthCoordinator(coord),
//	        httpclient.WithLogger(logger),
//	    },
//	    cfg.HTTPHealth.Options()...,
//	)...)
func (h *HTTPHealthClient) Options() []httpclient.Option {
	if h == nil {
		return nil
	}

	var opts []httpclient.Option

	opts = slices.AppendIf(opts, h.ServiceName != "",
		httpclient.WithHealthServiceName(h.ServiceName))

	opts = slices.AppendIf(opts, h.RetryWindow > 0,
		httpclient.WithHealthRetryWindow(h.RetryWindow))

	opts = slices.AppendIf(opts, h.RetryThreshold > 0,
		httpclient.WithHealthRetryThreshold(h.RetryThreshold))

	opts = slices.AppendIf(opts, h.RetryMinSamples > 0,
		httpclient.WithHealthRetryMinSamples(h.RetryMinSamples))

	opts = slices.AppendIf(opts, h.RetryBuckets > 0,
		httpclient.WithHealthRetryBuckets(h.RetryBuckets))

	opts = slices.AppendIf(opts, h.PerHost,
		httpclient.WithPerHostHealthChecks())

	return opts
}

// Options returns gRPC client options for health monitoring.
// The returned options should be used together with WithHealthCoordinator.
//
// Returns nil if h is nil, allowing optional health configuration.
//
// Example:
//
//	client, err := grpcclient.New(ctx, target, append(
//	    []grpcclient.Option{
//	        grpcclient.WithHealthCoordinator(coord),
//	        grpcclient.WithLogger(logger),
//	    },
//	    cfg.GRPCHealth.Options()...,
//	)...)
func (h *GRPCHealthClient) Options() []grpcclient.Option {
	if h == nil {
		return nil
	}

	var opts []grpcclient.Option

	opts = slices.AppendIf(opts, h.ServiceName != "",
		grpcclient.WithHealthServiceName(h.ServiceName))

	// Note: StateMapper requires a function, not a string.
	// The actual mapper implementation would need to be provided
	// by the caller or through a factory method.
	// For now, we only set the service name.

	return opts
}
