// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config //nolint:dupl

import validation "github.com/go-ozzo/ozzo-validation/v4"

// InterceptorsConfig defines the configuration for gRPC interceptors.
// Contains settings for various middleware components in the gRPC interceptor chain.
type InterceptorsConfig struct {
	// Cache contains configuration for response caching interceptor.
	Cache *GrpcInterCacheConfig `yaml:"cache" default:"-"`
	// RealIp contains configuration for real IP extraction interceptor.
	RealIp *GrpcInterRealIpConfig `yaml:"realIp" default:"-"`
	// RequestId contains configuration for request ID generation interceptor.
	RequestId *GrpcInterRequestIdConfig `yaml:"requestId" default:"-"`
	// Recovery contains configuration for panic recovery interceptor.
	Recovery *GrpcInterRecoveryConfig `yaml:"recovery" default:"-"`
	// Idempotency contains configuration for idempotency interceptor.
	Idempotency *GrpcInterIdempotencyConfig `yaml:"idempotency" default:"-"`
	// Auth contains configuration for authentication interceptor.
	Auth *GrpcInterAuthConfig `yaml:"auth" default:"-"`
	// Health contains configuration for health check interceptor.
	Health *GrpcInterHealthConfig `yaml:"health" default:"-"`
	// IpAcl contains configuration for IP access control interceptor.
	IpAcl *GrpcInterIpAclConfig `yaml:"ipAcl" default:"-"`
	// GeoAcl contains configuration for geographic access control interceptor.
	GeoAcl *GrpcInterGeoAclConfig `yaml:"geoAcl" default:"-"`
	// Limiter contains configuration for rate limiting interceptor.
	Limiter *GrpcInterLimiterConfig `yaml:"limiter" default:"-"`
	// Logger contains configuration for logging interceptor.
	Logger *GrpcInterLoggerConfig `yaml:"logger" default:"-"`
	// Metrics contains configuration for the metrics interceptor.
	Metrics *GrpcInterMetricsConfig `yaml:"metrics" default:"-"`
	// Tracing contains configuration for distributed tracing interceptor.
	Tracing *GrpcInterTracingConfig `yaml:"tracing" default:"-"`
	// ErrStatus contains configuration for error status interceptor.
	ErrStatus *GrpcInterErrStatusConfig `yaml:"errStatus" default:"-"`
}

// Validate performs validation of the InterceptorsConfig.
// Returns an error if validation fails, nil otherwise.
func (c *InterceptorsConfig) Validate() error {
	return ValidateStruct(c,
		validation.Field(&c.Cache, validation.NilOrNotEmpty),
		validation.Field(&c.RealIp, validation.NilOrNotEmpty),
		validation.Field(&c.RequestId, validation.NilOrNotEmpty),
		validation.Field(&c.Recovery, validation.NilOrNotEmpty),
		validation.Field(&c.Idempotency, validation.NilOrNotEmpty),
		validation.Field(&c.Auth, validation.NilOrNotEmpty),
		validation.Field(&c.Health, validation.NilOrNotEmpty),
		validation.Field(&c.IpAcl, validation.NilOrNotEmpty),
		validation.Field(&c.GeoAcl, validation.NilOrNotEmpty),
		validation.Field(&c.Limiter, validation.NilOrNotEmpty),
		validation.Field(&c.Logger, validation.NilOrNotEmpty),
		validation.Field(&c.Metrics, validation.NilOrNotEmpty),
		validation.Field(&c.Tracing, validation.NilOrNotEmpty),
		validation.Field(&c.ErrStatus, validation.NilOrNotEmpty),
	)
}

// DefaultInterceptorsConfig returns an InterceptorsConfig with default values for all interceptors.
// All interceptors are disabled by default.
func DefaultInterceptorsConfig() InterceptorsConfig {
	cache := DefaultGrpcInterCacheConfig()
	realIp := DefaultGrpcInterRealIpConfig()
	requestId := DefaultGrpcInterRequestIdConfig()
	recovery := DefaultGrpcInterRecoveryConfig()
	idempotency := DefaultGrpcInterIdempotencyConfig()
	auth := DefaultGrpcInterAuthConfig()
	hlth := DefaultGrpcInterHealthConfig()
	ipAcl := DefaultGrpcInterIpAclConfig()
	geoAcl := DefaultGrpcInterGeoAclConfig()
	limiter := DefaultGrpcInterLimiterConfig()
	logger := DefaultGrpcInterLoggerConfig()
	metricsCfg := DefaultGrpcInterMetricsConfig()
	tracing := DefaultGrpcInterTracingConfig()
	errStatus := DefaultGrpcInterErrStatusConfig()

	return InterceptorsConfig{
		Cache:       &cache,
		RealIp:      &realIp,
		RequestId:   &requestId,
		Recovery:    &recovery,
		Idempotency: &idempotency,
		Auth:        &auth,
		Health:      &hlth,
		IpAcl:       &ipAcl,
		GeoAcl:      &geoAcl,
		Limiter:     &limiter,
		Logger:      &logger,
		Metrics:     &metricsCfg,
		Tracing:     &tracing,
		ErrStatus:   &errStatus,
	}
}
