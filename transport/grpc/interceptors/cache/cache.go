// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/altessa-s/go-atlas/core/text/strings"
	"github.com/altessa-s/go-atlas/data/cache"
	"github.com/altessa-s/go-atlas/data/cache/providers"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/driver"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/metadata"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	slogx "github.com/altessa-s/go-atlas/observability/slog"
	grpcmetadata "google.golang.org/grpc/metadata"
)

// Cacher is an alias for cache.Cacher from the data/cache package.
// It defines the interface for high-level cache operations.
// Implementations must be thread-safe for concurrent access from multiple goroutines.
type Cacher = cache.Cacher

// cachedEntry represents a serialized cache entry containing either response data or error information.
// This internal type is used for JSON serialization of cached responses and errors.
type cachedEntry struct {
	Data      []byte     `json:"data,omitempty"`
	ErrorCode codes.Code `json:"error_code,omitempty"`
	ErrorMsg  string     `json:"error_msg,omitempty"`
}

// ServerInterceptor creates a gRPC server interceptor for response caching.
// The interceptor caches successful responses based on method configuration and applies
// compression when beneficial for large payloads.
//
// The cacher parameter must be thread-safe as it will be accessed concurrently.
// Use any Cacher implementation from github.com/altessa-s/go-atlas/data/cache.
//
// Options configure method-specific caching behavior, compression settings, and
// cache decision functions.
//
// Returns a configured interceptor that can be used with grpc.NewServer.
// Supports unary gRPC methods using the driven interceptor pattern.
func ServerInterceptor(cacher Cacher, opt ...Option) interceptors.ServerInterceptor {
	opts := newOptions(opt...)
	i := &interceptor{
		BaseInterceptor: interceptors.NewBaseInterceptorWithFilter(
			"cache",
			opts.ignoreMethods,
			opts.ignorePatterns,
			opts.logger,
		),
		cacher: cacher,
		opts:   opts,
	}
	return interceptors.ServerDrivenInterceptor(i)
}

type interceptor struct {
	interceptors.BaseInterceptor
	cacher Cacher
	opts   *options
}

// Ensure interceptor implements DrivenInterceptor
var _ driver.DrivenInterceptor = (*interceptor)(nil)
var _ interceptors.Interceptor = (*interceptor)(nil)

// Dependencies returns interceptors that cache reads from context.
// Cache depends on metadata for call information extraction.
func (i *interceptor) Dependencies() []string {
	return []string{"metadata"}
}

// DrivenInterceptor implements the driver.DrivenInterceptor interface.
// It creates a per-call driver instance for handling cache operations during RPC execution.
// The driver handles unary requests with intelligent caching strategies.
func (i *interceptor) DrivenInterceptor(ctx context.Context) (driver.Driver, context.Context) {
	// Metadata is guaranteed to be in context by the chain
	meta, _ := metadata.FromContext(ctx)

	// Check if method is cacheable
	config, ok := i.opts.methods[strings.InternLowerString(meta.FullyMethodName)]
	if !ok {
		return driver.NoopDriver(), ctx
	}

	// Check if this method should be ignored globally
	if i.ShouldIgnore(meta.FullyMethodName) {
		return driver.NoopDriver(), ctx
	}

	return &cacheDriver{
		interceptor: i,
		meta:        meta,
		config:      config,
		logger:      i.Logger(),
	}, ctx
}

// cacheResponse stores a successful response or error in the cache with the specified TTL.
// The method handles both success and error cases, serializing them into a common format
// for storage. It supports method-specific compression settings when available.
// Serialization errors are logged but do not affect the main request flow
// to ensure graceful degradation when caching fails.
func (i *interceptor) cacheResponse(ctx context.Context, key string, resp any, err error, ttl time.Duration, config *MethodConfig) {
	// Check for context cancellation before doing any work
	select {
	case <-ctx.Done():
		return // Don't cache if context is canceled
	default:
	}

	key = i.buildKey(key)

	if err != nil {
		// Cache error response
		if st, ok := status.FromError(err); ok {
			errorEntry := cachedEntry{ErrorCode: st.Code(), ErrorMsg: st.Message()}
			if saveErr := i.cacher.Save(ctx, key, errorEntry, ttl); saveErr != nil {
				i.Logger().Error("failed to save error entry to cache",
					slog.String("key", key), slogx.Error(saveErr))
			}
		}
		return
	}

	if resp != nil {
		// Check if context is canceled before expensive serialization
		select {
		case <-ctx.Done():
			return // Don't cache if context is canceled
		default:
		}

		// Use method-specific serialization
		data, marshalErr := i.opts.serializer.MarshalForMethod(ctx, resp, config)
		if marshalErr != nil {
			i.Logger().Error("failed to serialize response",
				slog.String("key", key), slogx.Error(marshalErr))
			return
		}

		successEntry := cachedEntry{Data: data}
		if saveErr := i.cacher.Save(ctx, key, successEntry, ttl); saveErr != nil {
			i.Logger().Error("failed to save entry to cache",
				slog.String("key", key), slogx.Error(saveErr))
		}
	}
}

// getCachedResponse retrieves and deserializes a cached response for the given key.
// Returns the deserialized response, any cached error, and a boolean indicating
// whether a valid cache entry was found. Supports method-specific decompression.
//
// Cache misses, deserialization failures, or invalid entries return false for the found parameter.
func (i *interceptor) getCachedResponse(ctx context.Context, key string, config *MethodConfig) (any, error, bool) {
	key = i.buildKey(key)

	var entry cachedEntry
	err := i.cacher.Get(ctx, key, &entry)
	if err != nil {
		// Cache miss or error - don't log ErrMissing as it's expected behavior
		if !errors.Is(err, providers.ErrMissing) && !errors.Is(err, cache.ErrMissing) {
			i.Logger().Error("failed to get cached entry",
				slog.String("key", key), slogx.Error(err))
		}
		return nil, nil, false //nolint:nilnil // third bool return discriminates cache miss
	}

	// Return cached error
	if entry.ErrorCode != codes.OK {
		return nil, status.Error(entry.ErrorCode, entry.ErrorMsg), true
	}

	// Use method-specific deserialization
	resp, err := i.opts.serializer.UnmarshalForMethod(ctx, entry.Data, config)
	if err != nil {
		// Log deserialization failures - this could indicate cache corruption,
		// version mismatches, or serialization issues
		i.Logger().Error("failed to deserialize cached response",
			slog.String("key", key), slogx.Error(err))
		return nil, nil, false //nolint:nilnil // third bool return discriminates cache miss
	}

	return resp, nil, true
}

func (i *interceptor) buildKey(key string) string {
	if i.opts.keysPrefix == "" {
		return key
	}
	return i.opts.keysPrefix + key
}

// Interned cache control headers set by the cache interceptor's [cacheDriver.PreCall]
// method. When cache headers are enabled (see [WithCacheHeaders]), these values are
// injected into gRPC response metadata so clients can distinguish cache hits from misses.
var (
	// HeaderKey is the gRPC metadata key for the cache status header.
	HeaderKey = strings.InternString("x-cache")
	// HeaderHit is the metadata value indicating a cache hit.
	HeaderHit = strings.InternString("hit")
	// HeaderMiss is the metadata value indicating a cache miss.
	HeaderMiss = strings.InternString("miss")
)

// cacheDriver implements the Driver interface for per-call cache operations.
// It handles unary RPC patterns with intelligent caching strategies.
// All fields except immutable ones (interceptor, meta, config) are protected by mutex.
type cacheDriver struct {
	// Immutable fields - set once during creation, safe for concurrent read access
	interceptor *interceptor
	meta        *metadata.CallMetadata
	config      *MethodConfig

	logger *slog.Logger // Optional logger for debugging and monitoring

	key atomic.Value
}

// Ensure cacheDriver implements Driver interface
var _ driver.Driver = (*cacheDriver)(nil)

// PreCall is called before the RPC handler execution.
// For unary calls, it attempts to serve the response from cache.
// Streaming calls are not supported and will skip caching.
func (d *cacheDriver) PreCall(ctx context.Context, req any) (any, error) {
	// Check for context cancellation before doing any work
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Generate cache key for this call
	key, err := d.interceptor.opts.keyGenerator(ctx, strings.InternString(d.meta.FullyMethodName), req)
	if err != nil {
		// If key generation fails, skip caching but log the error for debugging
		// We don't return the error to avoid breaking the request flow
		d.logger.Error("failed to generate cache key",
			slog.String("key", key), slogx.Error(err))
		return nil, nil //nolint:nilnil
	}

	d.key.Store(key)

	// Skip caching for streaming calls
	if d.meta.IsStream {
		return nil, nil //nolint:nilnil
	}

	// For unary calls, try to get from cache
	cachedResp, cachedErr, found := d.interceptor.getCachedResponse(ctx, key, d.config)

	if d.interceptor.opts.cacheHeadersEnabled {
		headerValue := HeaderMiss
		if found {
			headerValue = HeaderHit
		}
		_ = grpc.SetHeader(ctx, grpcmetadata.Pairs(HeaderKey, headerValue)) //nolint:errcheck
	}

	return cachedResp, cachedErr
}

// PostCall is called after the RPC completes.
// For unary calls, it caches successful responses based on the decision function.
// Streaming calls are not cached and will return the original error.
func (d *cacheDriver) PostCall(ctx context.Context, resp any, err error) error {
	// Check for context cancellation - if canceled, don't do caching work
	select {
	case <-ctx.Done():
		return err // Return original error, not context error, to preserve RPC semantics
	default:
	}

	if d.meta.IsStream {
		return err
	}

	// Load the cache key generated in PreCall
	cacheKey := d.key.Load().(string) //nolint:errcheck

	// For cache misses, use decision function to determine if we should cache
	decision := d.interceptor.opts.cacheDecision(ctx, strings.InternString(d.meta.FullyMethodName), nil, resp, err)
	if decision.ShouldCache {
		d.interceptor.cacheResponse(ctx, cacheKey, resp, err, decision.TTL, d.config)
	}

	return err
}
