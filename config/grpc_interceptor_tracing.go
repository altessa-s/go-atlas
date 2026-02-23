// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

// GrpcInterTracingConfig defines the configuration for the tracing interceptor.
// Controls distributed tracing for gRPC requests.
type GrpcInterTracingConfig struct {
	// BaseGrpcInterceptorConfig provides common interceptor configuration.
	BaseGrpcInterceptorConfig `yaml:",inline"`

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
func (c *GrpcInterTracingConfig) IsEnabled() bool {
	return c != nil && c.Enable
}

// Validate performs validation of the tracing interceptor configuration.
func (c *GrpcInterTracingConfig) Validate() error {
	return c.ValidateBase()
}

// DefaultGrpcInterTracingConfig returns a GrpcInterTracingConfig with default values.
// Tracing interceptor is disabled by default.
func DefaultGrpcInterTracingConfig() GrpcInterTracingConfig {
	return GrpcInterTracingConfig{
		BaseGrpcInterceptorConfig: BaseGrpcInterceptorConfig{
			EnableMixin: EnableMixin{Enable: false},
		},
		RecordPayload:    false,
		RecordMetadata:   true,
		RecordEvents:     true,
		PropagateContext: true,
	}
}
