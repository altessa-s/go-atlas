// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpcconfig

import middlewareconfig "github.com/altessa-s/go-atlas/config/middleware"

// RequestIDInterceptor defines the configuration for request ID generation.
// Controls how request IDs are generated, validated, and propagated
// throughout the gRPC interceptor chain and service call graph.
type RequestIDInterceptor struct {
	// BaseInterceptor provides common interceptor configuration.
	BaseInterceptor `yaml:",inline"`

	// GenerateIfMissing controls whether to generate a request ID if none is provided.
	// When true, missing request IDs are automatically generated using UUIDs.
	// When false, requests without IDs proceed without modification.
	// Recommended: true for production systems to ensure complete tracing coverage.
	GenerateIfMissing bool `yaml:"generateIfMissing" default:"true"`
}

// IsEnabled returns true if request ID generation is enabled.
// This is a convenience method to check if the interceptor should be active.
func (c *RequestIDInterceptor) IsEnabled() bool {
	return c != nil && c.Enabled
}

// Validate performs validation of the request ID interceptor configuration.
// Validates base configuration including method filtering.
//
// Returns an error if validation fails, nil otherwise.
func (c *RequestIDInterceptor) Validate() error {
	return c.ValidateBase()
}

// DefaultRequestIDInterceptor returns a RequestIDInterceptor with default values.
// Request ID generation is disabled by default.
func DefaultRequestIDInterceptor() RequestIDInterceptor {
	return RequestIDInterceptor{
		BaseInterceptor: BaseInterceptor{
			EnableMixin: middlewareconfig.EnableMixin{Enabled: false},
		},
		GenerateIfMissing: true,
	}
}
