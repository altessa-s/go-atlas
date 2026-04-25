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
	// Used to populate errdetails.ErrorInfo.Domain in gRPC error responses.
	// Example: "myservice.example.com"
	Domain *string `yaml:"domain"`

	// CacheSize is the maximum number of entries in the error conversion cache.
	// Set to 0 to disable caching. Default: 1000
	CacheSize *int `yaml:"cacheSize"`

	// CacheDisabled completely disables error conversion caching.
	// Default: false
	CacheDisabled bool `yaml:"cacheDisabled"`

	// CacheOnlySentinel restricts caching to sentinel errors only.
	// Prevents unbounded memory growth from caching dynamic errors.
	// Default: false
	CacheOnlySentinel bool `yaml:"cacheOnlySentinel"`
}

// IsEnabled returns true if error status conversion is enabled.
func (c *GrpcInterErrStatusConfig) IsEnabled() bool { return c != nil && c.Enabled }

// Validate performs validation of the error status interceptor configuration.
func (c *GrpcInterErrStatusConfig) Validate() error {
	return c.ValidateBase()
}

// DefaultGrpcInterErrStatusConfig returns a GrpcInterErrStatusConfig with default values.
func DefaultGrpcInterErrStatusConfig() GrpcInterErrStatusConfig {
	return GrpcInterErrStatusConfig{
		BaseGrpcInterceptorConfig: BaseGrpcInterceptorConfig{
			EnableMixin: EnableMixin{Enabled: false},
		},
	}
}
