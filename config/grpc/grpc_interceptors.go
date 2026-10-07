// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpcconfig

import (
	validationconfig "github.com/altessa-s/go-atlas/config/validation"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Interceptors defines the configuration for gRPC interceptors.
// Contains settings for various middleware components in the gRPC interceptor chain.
type Interceptors struct {
	// Cache contains configuration for response caching interceptor.
	Cache *CacheInterceptor `yaml:"cache" default:"-"`
	// RealIp contains configuration for real IP extraction interceptor.
	RealIp *RealIPInterceptor `yaml:"realIp" default:"-"`
	// RequestId contains configuration for request ID generation interceptor.
	RequestId *RequestIDInterceptor `yaml:"requestId" default:"-"`
	// Recovery contains configuration for panic recovery interceptor.
	Recovery *RecoveryInterceptor `yaml:"recovery" default:"-"`
	// Idempotency contains configuration for idempotency interceptor.
	Idempotency *IdempotencyInterceptor `yaml:"idempotency" default:"-"`
	// Auth contains configuration for authentication interceptor.
	Auth *AuthInterceptor `yaml:"auth" default:"-"`
	// Health contains configuration for health check interceptor.
	Health *HealthInterceptor `yaml:"health" default:"-"`
	// IpAcl contains configuration for IP access control interceptor.
	IpAcl *IPACLInterceptor `yaml:"ipAcl" default:"-"`
	// GeoAcl contains configuration for geographic access control interceptor.
	GeoAcl *GeoACLInterceptor `yaml:"geoAcl" default:"-"`
	// Limiter contains configuration for rate limiting interceptor.
	Limiter *LimiterInterceptor `yaml:"limiter" default:"-"`
	// Logger contains configuration for logging interceptor.
	Logger *LoggerInterceptor `yaml:"logger" default:"-"`
	// Metrics contains configuration for the metrics interceptor.
	Metrics *MetricsInterceptor `yaml:"metrics" default:"-"`
	// Tracing contains configuration for distributed tracing interceptor.
	Tracing *TracingInterceptor `yaml:"tracing" default:"-"`
	// ErrStatus contains configuration for error status interceptor.
	ErrStatus *ErrStatusInterceptor `yaml:"errStatus" default:"-"`
}

// Validate performs validation of the Interceptors.
// Returns an error if validation fails, nil otherwise.
func (c *Interceptors) Validate() error {
	return validationconfig.ValidateStruct(c,
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

// DefaultInterceptors returns an Interceptors with default values for all interceptors.
// All interceptors are disabled by default.
func DefaultInterceptors() Interceptors {
	cache := DefaultCacheInterceptor()
	realIp := DefaultRealIPInterceptor()
	requestId := DefaultRequestIDInterceptor()
	recovery := DefaultRecoveryInterceptor()
	idempotency := DefaultIdempotencyInterceptor()
	auth := DefaultAuthInterceptor()
	hlth := DefaultHealthInterceptor()
	ipAcl := DefaultIPACLInterceptor()
	geoAcl := DefaultGeoACLInterceptor()
	limiter := DefaultLimiterInterceptor()
	logger := DefaultLoggerInterceptor()
	metricsCfg := DefaultMetricsInterceptor()
	tracing := DefaultTracingInterceptor()
	errStatus := DefaultErrStatusInterceptor()

	return Interceptors{
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
