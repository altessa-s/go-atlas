// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate --type=options

import (
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/cache/compression"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/defaults"
)

const (
	DefaultTTL = 5 * time.Minute
)

const (
	minTTL = 1 * time.Second // Minimum reasonable TTL
	maxTTL = 24 * time.Hour  // Maximum allowed TTL (1 day)

	// bestPresetMinSizeDivisor is the divisor for minimum size in best compression preset.
	// Using half the default minimum size allows more aggressive compression.
	bestPresetMinSizeDivisor = 2

	// compressionLevelFast is gzip level 1, optimized for speed.
	compressionLevelFast = 1
	// compressionLevelBest is gzip level 9, optimized for compression ratio.
	compressionLevelBest = 9
)

// Use defaults package for optgen code generation
var _ = defaults.IgnorePatterns

// CompressionPreset is an alias for [compression.Preset] for easier access.
type CompressionPreset = compression.Preset

const (
	PresetNone     = compression.PresetNone
	PresetFast     = compression.PresetFast
	PresetBalanced = compression.PresetBalanced
	PresetBest     = compression.PresetBest
)

// MethodConfig defines caching configuration for a specific gRPC method.
// It contains the response prototype for unmarshaling, cache TTL duration,
// and compression settings for this specific method.
// This internal type is used to store per-method configuration.
type MethodConfig struct {
	Name          string // Name of the gRPC method (e.g., "/api.UserService/GetUser")
	ResponseProto any    // responseProto is a prototype of the response message for unmarshaling
	// compressor is the Compressor to use for this method (if UseCompression is true)
	compressor        compression.Compressor
	CompressionPreset compression.Preset
}

// Compressor returns the Compressor configured for this method.
func (com *MethodConfig) Compressor() compression.Compressor {
	if com.compressor == nil {
		// Default to no-op compressor if none is set
		return compression.NewNoOpCompressor()
	}
	return com.compressor
}

// HasCompressor checks if this method has a compressor configured.
func (com *MethodConfig) HasCompressor() bool {
	return com.compressor != nil && com.compressor != compression.NewNoOpCompressor()
}

// options configures the cache interceptor behavior and settings.
// This internal type contains all configuration options including method-specific settings,
// key generation, compression, and cache decision functions.
// Options are applied using the functional options pattern.
type options struct {
	// methods contains per-method cache configurations
	methods map[string]*MethodConfig `optgen:"default=make(map[string]*MethodConfig)"`
	// keyGenerator generates cache keys
	keyGenerator KeyGenerator `optgen:"default=DefaultKeyGenerator"`
	// cacheHeadersEnabled adds cache hit/miss headers to responses
	cacheHeadersEnabled bool `optgen:"default=true"`
	// serializer handles response serialization/deserialization
	serializer Serializer `optgen:"default=NewDefaultSerializer(nil)"`
	// cacheDecision is the function that decides whether to cache a response
	cacheDecision DecisionFunc `optgen:"default=DefaultSuccessOnlyDecision(DefaultTTL)"`
	// cacheTTL is the default TTL for cached responses
	cacheTTL time.Duration `optgen:"default=DefaultTTL"`
	// keysPrefix is an optional prefix for cache keys, useful for namespacing
	keysPrefix string
	// logger is an optional logger for debugging and monitoring
	logger *slog.Logger
	// ignoreMethods contains methods to skip caching for
	ignoreMethods []string
	// ignorePatterns contains regex patterns to skip caching for
	ignorePatterns []*regexp.Regexp `optgen:"default=defaults.IgnorePatterns"`
	// metadataKeys is a list of gRPC metadata keys to include in cache hash
	metadataKeys []string `opt:"-"`
	// metadataProcessor is a custom function to process metadata for hashing
	metadataProcessor MetadataProcessor `opt:"-"`
}

// WithMetadataKeys sets the gRPC metadata keys to include in the cache key hash.
// If not set, DefaultMetadataKeys will be used.
func WithMetadataKeys(keys ...string) Option {
	return func(o *options) {
		o.metadataKeys = keys
		// Update key generator to use new keys
		o.keyGenerator = NewKeyGenerator(o.metadataKeys, o.metadataProcessor)
	}
}

// WithMetadataProcessor sets a custom metadata processor for cache key generation.
// This allows for advanced logic such as parsing JWT tokens and including their claims in the hash.
func WithMetadataProcessor(p MetadataProcessor) Option {
	return func(o *options) {
		o.metadataProcessor = p
		// Update key generator to use new processor
		o.keyGenerator = NewKeyGenerator(o.metadataKeys, o.metadataProcessor)
	}
}

// WithCacheHeaders controls whether cache hit/miss headers are added to responses.
// When enabled, responses include an "x-cache" header with values "hit" or "miss".
// This is useful for debugging and monitoring cache behavior in development and production.
func WithCacheHeaders(enabled bool) Option {
	return func(o *options) {
		o.cacheHeadersEnabled = enabled
	}
}

// WithDefaultTTL sets the default cache TTL for all methods.
func WithDefaultTTL(ttl time.Duration) Option {
	return func(o *options) {
		if ttl < minTTL || ttl > maxTTL {
			ttl = DefaultTTL // Use default TTL for invalid values
		}
		o.cacheTTL = ttl
	}
}

// WithMethod adds a new method configuration with the specified method name and response prototype.
// The responseProto is used for unmarshaling cached responses.
func WithMethod(method string, responseProto any) Option {
	return func(o *options) {
		// Skip invalid configurations to prevent runtime errors
		if method == "" || responseProto == nil {
			return
		}

		o.methods[strings.ToLower(method)] = &MethodConfig{
			Name:          method,
			ResponseProto: responseProto,
		}
	}
}

// WithMethodConfig adds multiple method configurations using the provided Method structs.
// Each Method struct contains the method name, response prototype, TTL, and compression preset.
// This allows for bulk configuration of multiple methods in a single call.
func WithMethodConfig(method ...MethodConfig) Option {
	return func(o *options) {
		for _, m := range method {
			o.methods[strings.ToLower(m.Name)] = &MethodConfig{
				Name:              m.Name,
				ResponseProto:     m.ResponseProto,
				CompressionPreset: m.CompressionPreset,
				compressor:        compressorByPreset(m.CompressionPreset),
			}
		}
	}
}

func compressorByPreset(preset compression.Preset) compression.Compressor {
	switch preset {
	case compression.PresetFast:
		return compression.NewCompressor(compression.DefaultMinSize, compression.DefaultMaxSize, compressionLevelFast)
	case compression.PresetBalanced:
		return compression.NewCompressor(compression.DefaultMinSize, compression.DefaultMaxSize, compression.DefaultLevel)
	case compression.PresetBest:
		return compression.NewCompressor(compression.DefaultMinSize/bestPresetMinSizeDivisor, compression.DefaultMaxSize, compressionLevelBest)
	case compression.PresetNone:
		return compression.NewNoOpCompressor()
	default:
		return compression.NewNoOpCompressor()
	}
}

// WithCompression enables gzip compression with custom size and level settings.
// The minSize parameter sets the minimum size (in bytes) to trigger compression.
// The maxSize parameter sets the maximum size to allow compression (0 = no limit).
// The level parameter sets the gzip compression level (1-9, higher = better compression).
//
// Compression is automatically applied to responses that benefit from it based on size thresholds.
func WithCompression(minSize, maxSize, level int) Option {
	return func(o *options) {
		// Use gzip compression (only supported algorithm)
		compressor := compression.NewCompressor(minSize, maxSize, level)
		o.serializer = NewDefaultSerializer(compressor)
	}
}

// WithCompressionPreset enables gzip compression using predefined performance-tuned presets.
// Available presets:
//   - compression.PresetFast: Level 1, optimized for CPU-constrained environments
//   - compression.PresetBalanced: Level 6, optimal balance of speed and compression
//   - compression.PresetBest: Level 9, maximum compression for bandwidth-constrained environments
//   - compression.PresetNone: Disables compression
//
// Presets provide sensible defaults for common deployment scenarios without requiring
// detailed compression parameter tuning.
func WithCompressionPreset(preset compression.Preset) Option {
	return func(o *options) {
		o.serializer = NewDefaultSerializer(compressorByPreset(preset))
	}
}
