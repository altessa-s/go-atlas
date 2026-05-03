// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config //nolint:dupl

import validation "github.com/go-ozzo/ozzo-validation/v4"

// MiddlewaresConfig defines the configuration for HTTP middlewares.
// Contains settings for various middleware components in the HTTP server pipeline.
type MiddlewaresConfig struct {
	// BodyLimit contains configuration for request body size limiting middleware.
	BodyLimit *HttpInterBodyLimitConfig `yaml:"bodyLimit" default:"-"`
	// Cors contains configuration for CORS middleware.
	Cors *HttpInterCorsConfig `yaml:"cors" default:"-"`
	// Idempotency contains configuration for idempotency middleware.
	Idempotency *HttpInterIdempotencyConfig `yaml:"idempotency" default:"-"`
	// IpAcl contains configuration for IP access control middleware.
	IpAcl *HttpInterIpAclConfig `yaml:"ipAcl" default:"-"`
	// GeoAcl contains configuration for geographic access control middleware.
	GeoAcl *HttpInterGeoAclConfig `yaml:"geoAcl" default:"-"`
	// Limiter contains configuration for rate limiting middleware.
	Limiter *HttpInterLimiterConfig `yaml:"limiter" default:"-"`
	// Logger contains configuration for logging middleware.
	Logger *HttpInterLoggerConfig `yaml:"logger" default:"-"`
	// Metrics contains configuration for the metrics middleware.
	Metrics *HttpInterMetricsConfig `yaml:"metrics" default:"-"`
	// RealIp contains configuration for real IP extraction middleware.
	RealIp *HttpInterRealIpConfig `yaml:"realIp" default:"-"`
	// Recovery contains configuration for panic recovery middleware.
	Recovery *HttpInterRecoveryConfig `yaml:"recovery" default:"-"`
	// RequestId contains configuration for request ID middleware.
	RequestId *HttpInterRequestIdConfig `yaml:"requestId" default:"-"`
	// SecurityHeaders contains configuration for security headers middleware.
	SecurityHeaders *HttpInterSecurityHeadersConfig `yaml:"securityHeaders" default:"-"`
	// Tracing contains configuration for distributed tracing middleware.
	Tracing *HttpInterTracingConfig `yaml:"tracing" default:"-"`
}

// Validate performs validation of the MiddlewaresConfig.
// Returns an error if validation fails, nil otherwise.
func (c *MiddlewaresConfig) Validate() error {
	return ValidateStruct(c,
		validation.Field(&c.BodyLimit, validation.NilOrNotEmpty),
		validation.Field(&c.Cors, validation.NilOrNotEmpty),
		validation.Field(&c.Idempotency, validation.NilOrNotEmpty),
		validation.Field(&c.IpAcl, validation.NilOrNotEmpty),
		validation.Field(&c.GeoAcl, validation.NilOrNotEmpty),
		validation.Field(&c.Limiter, validation.NilOrNotEmpty),
		validation.Field(&c.Logger, validation.NilOrNotEmpty),
		validation.Field(&c.Metrics, validation.NilOrNotEmpty),
		validation.Field(&c.RealIp, validation.NilOrNotEmpty),
		validation.Field(&c.Recovery, validation.NilOrNotEmpty),
		validation.Field(&c.RequestId, validation.NilOrNotEmpty),
		validation.Field(&c.SecurityHeaders, validation.NilOrNotEmpty),
		validation.Field(&c.Tracing, validation.NilOrNotEmpty),
	)
}

// DefaultMiddlewaresConfig returns a MiddlewaresConfig with default values for all middlewares.
// All middlewares are disabled by default.
func DefaultMiddlewaresConfig() MiddlewaresConfig {
	bodyLimit := DefaultHttpInterBodyLimitConfig()
	cors := DefaultHttpInterCorsConfig()
	idempotency := DefaultHttpInterIdempotencyConfig()
	ipAcl := DefaultHttpInterIpAclConfig()
	geoAcl := DefaultHttpInterGeoAclConfig()
	limiter := DefaultHttpInterLimiterConfig()
	logger := DefaultHttpInterLoggerConfig()
	metricsCfg := DefaultHttpInterMetricsConfig()
	realIp := DefaultHttpInterRealIpConfig()
	recovery := DefaultHttpInterRecoveryConfig()
	requestId := DefaultHttpInterRequestIdConfig()
	securityHeaders := DefaultHttpInterSecurityHeadersConfig()
	tracing := DefaultHttpInterTracingConfig()

	return MiddlewaresConfig{
		BodyLimit:       &bodyLimit,
		Cors:            &cors,
		Idempotency:     &idempotency,
		IpAcl:           &ipAcl,
		GeoAcl:          &geoAcl,
		Limiter:         &limiter,
		Logger:          &logger,
		Metrics:         &metricsCfg,
		RealIp:          &realIp,
		Recovery:        &recovery,
		RequestId:       &requestId,
		SecurityHeaders: &securityHeaders,
		Tracing:         &tracing,
	}
}
