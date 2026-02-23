// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"errors"
	"fmt"
	"net/netip"

	validation "github.com/go-ozzo/ozzo-validation/v4"
)

const (
	// defaultRealIPCacheSize is the default size of the IP validation cache.
	defaultRealIPCacheSize = 1000
)

// GrpcInterRealIpConfig defines the configuration for real IP extraction from proxy headers.
// Used by the realip interceptor to extract client IP addresses from various
// proxy headers while validating trust relationships with intermediate proxies.
type GrpcInterRealIpConfig struct {
	// BaseGrpcInterceptorConfig provides common interceptor configuration.
	BaseGrpcInterceptorConfig `yaml:",inline"`

	// TrustedProxies is a list of trusted proxy IP addresses or CIDR blocks.
	// Only requests from these proxies will have their X-Forwarded-For headers processed.
	// Example: ["10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"]
	TrustedProxies []string `yaml:"trustedProxies"`

	// TrustedProxiesCount specifies the number of trusted proxies that may append X-Forwarded-For.
	// This helps prevent IP spoofing by limiting the number of hops we trust.
	// Set to 0 to trust unlimited proxy hops (not recommended for production).
	TrustedProxiesCount uint `yaml:"trustedProxiesCount" default:"0"`

	// TrustedPeers is a list of trusted peer IP addresses or CIDR blocks.
	// These are additional trusted sources beyond the standard proxy list.
	// Useful for direct connections from known services or clusters.
	TrustedPeers []string `yaml:"trustedPeers"`

	// Headers is a list of headers to check for real IP (in order of preference).
	// The interceptor will check these headers in order until a valid IP is found.
	// Default headers: X-Forwarded-For, X-Real-IP, X-Client-IP, CF-Connecting-IP,
	// Fastly-Client-Ip, True-Client-Ip.
	Headers []string `yaml:"headers"`

	// CacheSize specifies the size of the IP validation cache (0 to disable).
	// Caching improves performance by avoiding repeated IP prefix parsing.
	// Recommended values: 1000-10000 for high-traffic services.
	CacheSize int `yaml:"cacheSize" default:"1000"`
}

// IsEnabled returns true if real IP extraction is enabled.
// This is a convenience method to check if the interceptor should be active.
func (c *GrpcInterRealIpConfig) IsEnabled() bool {
	return c != nil && c.Enable
}

// Validate performs validation of the real IP interceptor configuration.
// Ensures all IP addresses and CIDR blocks are valid and configuration is sane.
//
// Validation rules:
//   - TrustedProxies: each entry must be a valid IP address or CIDR prefix
//   - TrustedPeers: each entry must be a valid IP address or CIDR prefix
//   - Headers: each header name must be non-empty when specified
//   - CacheSize: must be non-negative
//   - IgnoreMethods: each method name must be non-empty when specified
//   - IgnorePatterns: each pattern must be non-empty when specified
//
// Returns an error if validation fails, nil otherwise.
func (c *GrpcInterRealIpConfig) Validate() error {
	return c.ValidateBase(func() error {
		return ValidateStruct(c,
			validation.Field(&c.TrustedProxies, validation.Each(validation.By(validateIP))),
			validation.Field(&c.TrustedPeers, validation.Each(validation.By(validateIP))),
			validation.Field(&c.Headers, validation.Each(validation.Required)),
			validation.Field(&c.CacheSize, validation.Min(0)),
		)
	})
}

// DefaultGrpcInterRealIpConfig returns a GrpcInterRealIpConfig with default values.
// Real IP extraction is disabled by default.
func DefaultGrpcInterRealIpConfig() GrpcInterRealIpConfig {
	return GrpcInterRealIpConfig{
		BaseGrpcInterceptorConfig: BaseGrpcInterceptorConfig{
			EnableMixin: EnableMixin{Enable: false},
		},
		TrustedProxiesCount: 0,
		CacheSize:           defaultRealIPCacheSize,
	}
}

// validateIP validates that a value is a valid IP address or CIDR prefix.
// This function ensures that trusted proxy and peer entries are properly formatted.
func validateIP(value any) error {
	str, ok := value.(string)
	if !ok {
		// Should not happen with struct validation
		return errors.New("invalid type for IP validation")
	}
	if _, err := netip.ParsePrefix(str); err != nil {
		return fmt.Errorf("invalid IP: '%s'", str)
	}
	return nil
}
