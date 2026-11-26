// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

//go:generate go tool optgen generate --type=options

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

// MethodPolicy is the caching policy of one gRPC method: the response
// prototype used for unmarshaling and the compression applied to its cached
// responses. It is a per-method record, registered with [WithMethodPolicy] or
// [WithMethods] and handed to a [Serializer] on every cached call.
type MethodPolicy struct {
	Name          string // Name of the gRPC method (e.g., "/api.UserService/GetUser")
	ResponseProto any    // responseProto is a prototype of the response message for unmarshaling
	// compressor is the Compressor to use for this method (if UseCompression is true)
	compressor        compression.Compressor
	CompressionPreset compression.Preset
}

// Compressor returns the Compressor configured for this method.
func (com *MethodPolicy) Compressor() compression.Compressor {
	if com.compressor == nil {
		// Default to no-op compressor if none is set
		return compression.NewNoOpCompressor()
	}
	return com.compressor
}

// HasCompressor checks if this method has a compressor configured.
func (com *MethodPolicy) HasCompressor() bool {
	return com.compressor != nil && com.compressor != compression.NewNoOpCompressor()
}

// options configures the cache interceptor behavior and settings.
// This internal type contains all configuration options including method-specific settings,
// key generation, compression, and cache decision functions.
// Options are applied using the functional options pattern.
type options struct {
	// methods contains per-method cache configurations
	methods map[string]*MethodPolicy `optgen:"default=make(map[string]*MethodPolicy)"`
	// keyGenerator overrides only the storage key. Identity checks and
	// verification of the full request tuple still apply.
	keyGenerator KeyGenerator
	// identityFunc resolves the verified caller's cache namespace.
	identityFunc IdentityFunc `optgen:"default=defaultIdentity,notnil"`
	// authRequired bypasses caching when no verified caller is identifiable.
	authRequired bool
	// WithCacheHeaders controls whether cache hit/miss headers are added to responses.
	// When enabled, responses include an "x-cache" header with values "hit" or "miss".
	// This is useful for debugging and monitoring cache behavior in development and production.
	cacheHeadersEnabled bool `opt:"CacheHeaders" optval:"param" optgen:"default=true"`
	// serializer handles response serialization/deserialization
	serializer Serializer `optgen:"default=NewDefaultSerializer(nil)"`
	// cacheDecision is the function that decides whether to cache a response.
	// When unset, ServerInterceptor uses DefaultSuccessOnlyDecision(cacheTTL).
	cacheDecision DecisionFunc
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
	// metadataKeys selects response discriminators. An empty list uses
	// DefaultMetadataKeys. The default read-mask header is always included.
	metadataKeys []string
	// metadataProcessor supplies additional response discriminators.
	metadataProcessor MetadataProcessor
	// readMaskHeader adds the configured read-mask header to the discriminators.
	readMaskHeader string
}

// WithCacheHeadersEnabled enables the cacheHeadersEnabled option.
//
// cacheHeadersEnabled adds cache hit/miss headers to responses
func WithCacheHeadersEnabled() Option {
	return func(o *options) { o.cacheHeadersEnabled = true }
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

		o.methods[strings.ToLower(method)] = &MethodPolicy{
			Name:          method,
			ResponseProto: responseProto,
		}
	}
}

// WithMethodPolicy registers the given per-method policies. Each [MethodPolicy]
// carries the method name, response prototype, and compression preset, so
// several methods can be registered in a single call.
func WithMethodPolicy(method ...MethodPolicy) Option {
	return func(o *options) {
		for _, m := range method {
			o.methods[strings.ToLower(m.Name)] = &MethodPolicy{
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
