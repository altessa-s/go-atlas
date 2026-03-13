// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

// GrpcInterErrStatusConfig defines the configuration for the error status interceptor.
// The interceptor converts application errors to gRPC status errors and enriches
// error responses with metadata such as domain and request information.
type GrpcInterErrStatusConfig struct {
	// BaseGrpcInterceptorConfig provides common interceptor configuration.
	BaseGrpcInterceptorConfig `yaml:",inline"`

	// Domain is the logical namespace for error reason codes of this service.
	Domain *string `yaml:"domain"`
}

// Validate performs validation of the error status interceptor configuration.
func (c *GrpcInterErrStatusConfig) Validate() error {
	return c.ValidateBase()
}

// DefaultGrpcInterErrStatusConfig returns a GrpcInterErrStatusConfig with default values.
func DefaultGrpcInterErrStatusConfig() GrpcInterErrStatusConfig {
	return GrpcInterErrStatusConfig{
		BaseGrpcInterceptorConfig: BaseGrpcInterceptorConfig{
			EnableMixin: EnableMixin{Enable: false},
		},
	}
}
