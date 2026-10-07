// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package httpconfig

import (
	validationconfig "github.com/altessa-s/go-atlas/config/validation"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Middlewares defines the configuration for HTTP middlewares.
// Contains settings for various middleware components in the HTTP server pipeline.
type Middlewares struct {
	// BodyLimit contains configuration for request body size limiting middleware.
	BodyLimit *BodyLimitMiddleware `yaml:"bodyLimit" default:"-"`
	// Cors contains configuration for CORS middleware.
	Cors *CORSMiddleware `yaml:"cors" default:"-"`
	// Idempotency contains configuration for idempotency middleware.
	Idempotency *IdempotencyMiddleware `yaml:"idempotency" default:"-"`
	// IpAcl contains configuration for IP access control middleware.
	IpAcl *IPACLMiddleware `yaml:"ipAcl" default:"-"`
	// GeoAcl contains configuration for geographic access control middleware.
	GeoAcl *GeoACLMiddleware `yaml:"geoAcl" default:"-"`
	// Limiter contains configuration for rate limiting middleware.
	Limiter *LimiterMiddleware `yaml:"limiter" default:"-"`
	// Logger contains configuration for logging middleware.
	Logger *LoggerMiddleware `yaml:"logger" default:"-"`
	// Metrics contains configuration for the metrics middleware.
	Metrics *MetricsMiddleware `yaml:"metrics" default:"-"`
	// RealIp contains configuration for real IP extraction middleware.
	RealIp *RealIPMiddleware `yaml:"realIp" default:"-"`
	// Recovery contains configuration for panic recovery middleware.
	Recovery *RecoveryMiddleware `yaml:"recovery" default:"-"`
	// RequestId contains configuration for request ID middleware.
	RequestId *RequestIDMiddleware `yaml:"requestId" default:"-"`
	// SecurityHeaders contains configuration for security headers middleware.
	SecurityHeaders *SecurityHeadersMiddleware `yaml:"securityHeaders" default:"-"`
	// Tracing contains configuration for distributed tracing middleware.
	Tracing *TracingMiddleware `yaml:"tracing" default:"-"`
}

// Validate performs validation of the Middlewares.
// Returns an error if validation fails, nil otherwise.
func (c *Middlewares) Validate() error {
	return validationconfig.ValidateStruct(c,
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

// DefaultMiddlewares returns a Middlewares with default values for all middlewares.
// All middlewares are disabled by default.
func DefaultMiddlewares() Middlewares {
	bodyLimit := DefaultBodyLimitMiddleware()
	cors := DefaultCORSMiddleware()
	idempotency := DefaultIdempotencyMiddleware()
	ipAcl := DefaultIPACLMiddleware()
	geoAcl := DefaultGeoACLMiddleware()
	limiter := DefaultLimiterMiddleware()
	logger := DefaultLoggerMiddleware()
	metricsCfg := DefaultMetricsMiddleware()
	realIp := DefaultRealIPMiddleware()
	recovery := DefaultRecoveryMiddleware()
	requestId := DefaultRequestIDMiddleware()
	securityHeaders := DefaultSecurityHeadersMiddleware()
	tracing := DefaultTracingMiddleware()

	return Middlewares{
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
