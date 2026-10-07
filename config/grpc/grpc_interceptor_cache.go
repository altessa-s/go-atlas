// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpcconfig

import (
	"time"

	middlewareconfig "github.com/altessa-s/go-atlas/config/middleware"
	validationconfig "github.com/altessa-s/go-atlas/config/validation"
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for CacheInterceptor.
const (
	defaultCacheSuccessTTL       = 24 * time.Hour
	defaultCacheErrorTTL         = 1 * time.Hour
	defaultCacheCompressionLevel = CacheCompressionPresetFast
)

// CacheCompressionPreset defines the compression levels available for cached responses.
// Different presets balance compression ratio vs CPU usage for optimal performance.
type CacheCompressionPreset string

const (
	// CacheCompressionPresetNone disables compression for cached responses.
	// Fastest option with no CPU overhead but largest storage usage.
	CacheCompressionPresetNone CacheCompressionPreset = "none"

	// CacheCompressionPresetFast provides a fast compression level for cached responses.
	// Good balance between speed and compression ratio for most use cases.
	CacheCompressionPresetFast CacheCompressionPreset = "fast"

	// CacheCompressionPresetBalanced provides a balanced compression level for cached responses.
	// Optimized for balanced CPU usage and compression efficiency.
	CacheCompressionPresetBalanced CacheCompressionPreset = "balanced"

	// CacheCompressionPresetBest provides the best compression level for cached responses.
	// Highest compression ratio but increased CPU usage for compression/decompression.
	CacheCompressionPresetBest CacheCompressionPreset = "best"
)

// CacheInterceptor defines the configuration for gRPC response caching.
// Controls TTL, compression, and header behavior for cached responses.
type CacheInterceptor struct {
	// BaseInterceptor provides common interceptor configuration.
	BaseInterceptor `yaml:",inline"`

	// SuccessTTL defines the default TTL for successful responses.
	// Successful responses (no gRPC errors) are cached for this duration.
	// Must be at least 1 minute to prevent cache thrashing.
	SuccessTTL time.Duration `yaml:"successTTL" default:"24h"`

	// ErrorTTL defines the default TTL for error responses.
	// Error responses are cached for shorter periods to allow quick recovery.
	// Must be at least 1 minute to prevent cache thrashing.
	ErrorTTL time.Duration `yaml:"errorTTL" default:"1h"`

	// EnableCacheHeaders controls whether cache hit/miss headers are added to responses.
	// When enabled, responses include headers indicating cache status for debugging.
	EnableCacheHeaders bool `yaml:"enableCacheHeaders" default:"true"`

	// CompressionPreset defines compression level for cached responses.
	// Higher compression reduces storage but increases CPU usage.
	CompressionPreset CacheCompressionPreset `yaml:"compressionPreset" default:"fast"`

	// KeysPrefix is a prefix for all cache keys in storage.
	// Useful for namespacing cache keys when sharing storage between services.
	KeysPrefix string `yaml:"keysPrefix"`

	// KeyMetadata is a list of gRPC metadata keys (headers) to include in the cache key hash.
	// If empty, a standard set of security and identity headers is used.
	KeyMetadata []string `yaml:"keyMetadata"`
}

// IsEnabled returns true if response caching is enabled.
// This is a convenience method to check if the interceptor should be active.
func (c *CacheInterceptor) IsEnabled() bool {
	return c != nil && c.Enabled
}

// Validate performs validation of the cache interceptor configuration.
// Ensures TTL values are reasonable and compression preset is valid.
//
// Validation rules:
//   - SuccessTTL: must be at least 1 minute when caching is enabled
//   - ErrorTTL: must be at least 1 minute when caching is enabled
//   - CompressionPreset: must be one of the defined preset values
//   - IgnoreMethods: each method name must be non-empty when specified
//   - IgnorePatterns: each pattern must be non-empty when specified
//
// Returns an error if validation fails, nil otherwise.
func (c *CacheInterceptor) Validate() error {
	return c.ValidateBase(func() error {
		return validationconfig.ValidateStruct(c,
			validation.Field(&c.SuccessTTL, validation.Required, ozzo_rules.Duration(), validation.Min(time.Minute)),
			validation.Field(&c.ErrorTTL, validation.Required, ozzo_rules.Duration(), validation.Min(time.Minute)),
			validation.Field(&c.CompressionPreset, validation.Required,
				ozzo_rules.OneOf(CacheCompressionPresetNone, CacheCompressionPresetFast,
					CacheCompressionPresetBalanced, CacheCompressionPresetBest)),
		)
	})
}

// DefaultCacheInterceptor returns a CacheInterceptor with default values.
// Caching is disabled by default.
func DefaultCacheInterceptor() CacheInterceptor {
	return CacheInterceptor{
		BaseInterceptor: BaseInterceptor{
			EnableMixin: middlewareconfig.EnableMixin{Enabled: false},
		},
		SuccessTTL:         defaultCacheSuccessTTL,
		ErrorTTL:           defaultCacheErrorTTL,
		EnableCacheHeaders: true,
		CompressionPreset:  defaultCacheCompressionLevel,
	}
}
