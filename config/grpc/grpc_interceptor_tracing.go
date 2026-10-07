// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpcconfig

import middlewareconfig "github.com/altessa-s/go-atlas/config/middleware"

// TracingInterceptor defines the configuration for the tracing interceptor.
// Controls distributed tracing for gRPC requests.
type TracingInterceptor struct {
	// BaseInterceptor provides common interceptor configuration.
	BaseInterceptor `yaml:",inline"`

	// RecordPayload determines whether request/response payloads are recorded.
	// Warning: This can significantly increase trace size and may expose sensitive data.
	// Defaults to false.
	RecordPayload bool `yaml:"recordPayload" default:"false"`

	// RecordMetadata determines whether gRPC metadata is recorded as span attributes.
	// Defaults to true.
	RecordMetadata bool `yaml:"recordMetadata" default:"true"`

	// RecordEvents determines whether streaming message events are recorded.
	// Defaults to true.
	RecordEvents bool `yaml:"recordEvents" default:"true"`

	// PropagateContext determines whether trace context is propagated from incoming requests.
	// When true, parent spans from incoming requests are linked to server spans.
	// Defaults to true.
	PropagateContext bool `yaml:"propagateContext" default:"true"`
}

// IsEnabled returns true if the tracing interceptor is enabled.
func (c *TracingInterceptor) IsEnabled() bool {
	return c != nil && c.Enabled
}

// Validate performs validation of the tracing interceptor configuration.
func (c *TracingInterceptor) Validate() error {
	return c.ValidateBase()
}

// DefaultTracingInterceptor returns a TracingInterceptor with default values.
// Tracing interceptor is disabled by default.
func DefaultTracingInterceptor() TracingInterceptor {
	return TracingInterceptor{
		BaseInterceptor: BaseInterceptor{
			EnableMixin: middlewareconfig.EnableMixin{Enabled: false},
		},
		RecordPayload:    false,
		RecordMetadata:   true,
		RecordEvents:     true,
		PropagateContext: true,
	}
}
