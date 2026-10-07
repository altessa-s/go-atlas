// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clienthealthconfig

import (
	"time"

	validationconfig "github.com/altessa-s/go-atlas/config/validation"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for Config configuration.
const (
	// DefaultRetryWindow is the default sliding window for retry rate calculation.
	DefaultRetryWindow = 60 * time.Second

	// DefaultRetryThreshold is the default retry rate threshold for degradation.
	DefaultRetryThreshold = 0.2

	// DefaultRetryMinSamples is the default minimum samples before retry rate affects health.
	DefaultRetryMinSamples = uint64(10)

	// DefaultRetryBuckets is the default number of buckets in the sliding window.
	DefaultRetryBuckets = 60

	// MaxHealthClientRetryBuckets is the maximum allowed number of buckets (1 hour with 1-second buckets).
	MaxHealthClientRetryBuckets = 3600
)

// StateMapper defines the mapping strategy from gRPC connection states to health status.
type StateMapper string

const (
	// StateMapperDefault uses the default mapping from connectivity.State to ServingStatus.
	StateMapperDefault StateMapper = "default"

	// StateMapperStrict maps any non-READY state to NOT_SERVING.
	StateMapperStrict StateMapper = "strict"

	// StateMapperLenient maps CONNECTING/IDLE to SERVING, only TRANSIENT_FAILURE/SHUTDOWN to NOT_SERVING.
	StateMapperLenient StateMapper = "lenient"
)

// Config contains base health monitoring configuration for external service clients.
// This is embedded in transport-specific health configurations.
type Config struct {
	// ServiceName is the name registered with the health coordinator.
	// This name appears in health status reports and metrics.
	// Required field.
	ServiceName string `yaml:"serviceName"`
}

// HTTP contains health monitoring configuration for HTTP clients.
// Health is derived from retry rate and circuit breaker state.
//
// Example:
//
//	health := &clienthealthconfig.HTTP{
//	    Config: clienthealthconfig.Config{
//	        ServiceName: "openai-api",
//	    },
//	    RetryWindow:     60 * time.Second,
//	    RetryThreshold:  0.15,
//	    RetryMinSamples: 5,
//	}
//
//	httpClient := httpclient.New(append(
//	    []httpclient.Option{httpclient.WithHealthCoordinator(coord)},
//	    factory.HealthOptions(health)...,
//	)...)
type HTTP struct {
	Config `yaml:",inline"`

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

// GRPC contains health monitoring configuration for gRPC clients.
// Health is derived from connection state.
//
// Example:
//
//	health := &clienthealthconfig.GRPC{
//	    Config: clienthealthconfig.Config{
//	        ServiceName: "user-service",
//	    },
//	    StateMapper: clienthealthconfig.StateMapperStrict,
//	}
//
//	grpcClient, _ := grpcclient.New(ctx, target, append(
//	    []grpcclient.Option{grpcclient.WithHealthCoordinator(coord)},
//	    factory.HealthOptions(health)...,
//	)...)
type GRPC struct {
	Config `yaml:",inline"`

	// StateMapper selects the mapping strategy from gRPC connection state to health status.
	// Defaults to "default".
	StateMapper StateMapper `yaml:"stateMapper" default:"default"`

	// PerTarget enables per-target health checks for connection pools.
	// When true, registers separate health checkers for each target address.
	PerTarget bool `yaml:"perTarget" default:"false"`
}

// Default returns a Config configuration with default values.
// Note: ServiceName must be set explicitly as it's a required field.
func Default() Config {
	return Config{}
}

// DefaultHTTP returns an HTTP configuration with default values.
// Note: ServiceName must be set explicitly as it's a required field.
func DefaultHTTP() HTTP {
	return HTTP{
		RetryWindow:     DefaultRetryWindow,
		RetryThreshold:  DefaultRetryThreshold,
		RetryMinSamples: DefaultRetryMinSamples,
		RetryBuckets:    DefaultRetryBuckets,
	}
}

// DefaultGRPC returns a GRPC configuration with default values.
// Note: ServiceName must be set explicitly as it's a required field.
func DefaultGRPC() GRPC {
	return GRPC{
		StateMapper: StateMapperDefault,
	}
}

// Validate performs validation of the Config configuration.
// Returns an error if validation fails, nil otherwise.
func (h *Config) Validate() error {
	return validationconfig.ValidateStruct(h,
		validation.Field(&h.ServiceName,
			validation.Required),
	)
}

// Validate performs validation of the HTTP configuration.
// Returns an error if validation fails, nil otherwise.
func (h *HTTP) Validate() error {
	if err := h.Config.Validate(); err != nil {
		return err
	}
	return validationconfig.ValidateStruct(h,
		validation.Field(&h.RetryWindow,
			validation.When(h.RetryWindow > 0, validation.Min(time.Second))),
		validation.Field(&h.RetryThreshold,
			validation.Min(0.0),
			validation.Max(1.0)),
		validation.Field(&h.RetryMinSamples,
			validation.When(h.RetryMinSamples > 0, validation.Min(uint64(1)))),
		validation.Field(&h.RetryBuckets,
			validation.When(h.RetryBuckets > 0, validation.Min(1), validation.Max(MaxHealthClientRetryBuckets))),
	)
}

// Validate performs validation of the GRPC configuration.
// Returns an error if validation fails, nil otherwise.
func (h *GRPC) Validate() error {
	if err := h.Config.Validate(); err != nil {
		return err
	}
	return validationconfig.ValidateStruct(h,
		validation.Field(&h.StateMapper,
			validation.In(
				StateMapper(""),
				StateMapperDefault,
				StateMapperStrict,
				StateMapperLenient,
			)),
	)
}
