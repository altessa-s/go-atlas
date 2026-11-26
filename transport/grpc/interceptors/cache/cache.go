// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas-core/text/strings/intern"
	"github.com/altessa-s/go-atlas/cache"
	"github.com/altessa-s/go-atlas/internal/diagnostics"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/driver"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/metadata"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	grpcmetadata "google.golang.org/grpc/metadata"
)

const interceptorName = "cache"

// Name returns the interceptor name used for dependency resolution and chain ordering.
func Name() string { return interceptorName }

// ID is a lightweight [interceptors.Interceptor] reference for this package,
// suitable for passing to exclusion lists.
var ID = interceptors.Ref(interceptorName)

// cachedEntry represents a serialized cache entry containing either response data or error information.
// This internal type is used for JSON serialization of cached responses and errors.
type cachedEntry struct {
	Verifier []byte `json:"verifier"`
	Data     []byte `json:"data,omitempty"`
	Status   []byte `json:"status,omitempty"`
}

// ServerInterceptor creates a gRPC server interceptor for response caching.
// The interceptor caches successful responses based on method configuration and applies
// compression when beneficial for large payloads.
//
// The cacher parameter must be thread-safe as it will be accessed concurrently.
// Use any Cacher implementation from github.com/altessa-s/go-atlas/cache.
//
// Options configure method-specific caching behavior, compression settings, and
// cache decision functions.
//
// Returns a configured interceptor that can be used with grpc.NewServer.
// Supports unary gRPC methods using the driven interceptor pattern.
func ServerInterceptor(cacher cache.Cacher, opt ...Option) interceptors.ServerInterceptor {
	opts := newOptions(opt...)
	// Build the fallback decision after all options ran so WithDefaultTTL /
	// WithCacheTTL take effect; an explicit WithCacheDecision is kept as is.
	if opts.cacheDecision == nil {
		opts.cacheDecision = DefaultSuccessOnlyDecision(opts.cacheTTL)
	}
	i := &interceptor{
		BaseInterceptor: interceptors.NewBaseInterceptorWithFilter(
			interceptorName,
			opts.ignoreMethods,
			opts.ignorePatterns,
			opts.logger,
		),
		cacher:         cacher,
		opts:           opts,
		discriminators: newDiscriminators(opts.metadataKeys, opts.metadataProcessor, opts.readMaskHeader),
	}
	return interceptors.ServerDrivenInterceptor(i)
}

type interceptor struct {
	interceptors.BaseInterceptor
	cacher         cache.Cacher
	opts           *options
	discriminators discriminators
}

// Ensure interceptor implements DrivenInterceptor
var _ driver.DrivenInterceptor = (*interceptor)(nil)
var _ interceptors.Interceptor = (*interceptor)(nil)

// Dependencies places cache hits after authentication and access controls.
// Each dependency is optional in a chain that does not install it.
func (i *interceptor) Dependencies() []string {
	return []string{metadata.Name(), "auth", "ipacl", "geoacl", "limiter"}
}

// DrivenInterceptor implements the driver.DrivenInterceptor interface.
// It creates a per-call driver instance for handling cache operations during RPC execution.
// The driver handles unary requests with intelligent caching strategies.
func (i *interceptor) DrivenInterceptor(ctx context.Context) (driver.Driver, context.Context) {
	// Metadata is guaranteed to be in context by the chain
	meta, _ := metadata.FromContext(ctx)

	// Check if method is cacheable
	config, ok := i.opts.methods[intern.LowerString(meta.FullyMethodName)]
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
func (i *interceptor) cacheResponse(ctx context.Context, key string, verifier []byte, resp any, err error, ttl time.Duration, config *MethodPolicy) {
	// Check for context cancellation before doing any work
	select {
	case <-ctx.Done():
		return // Don't cache if context is canceled
	default:
	}

	key = i.buildKey(key)

	entry := cachedEntry{Verifier: verifier}
	var marshalErr error
	switch {
	case err != nil:
		st, ok := status.FromError(err)
		if !ok {
			return
		}
		entry.Status, marshalErr = proto.Marshal(st.Proto())
	case resp != nil:
		entry.Data, marshalErr = i.opts.serializer.MarshalForMethod(ctx, resp, config)
	default:
		return
	}
	if marshalErr != nil {
		i.Logger().Error("failed to serialize response",
			slog.String("classification", diagnostics.ErrorClass(marshalErr)))
		return
	}
	if saveErr := i.cacher.Save(ctx, key, entry, ttl); saveErr != nil {
		i.Logger().Error("failed to save entry to cache",
			slog.String("classification", diagnostics.ErrorClass(saveErr)))
	}
}

// getCachedResponse retrieves and deserializes a cached response for the given key.
// Returns the deserialized response, any cached error, and a boolean indicating
// whether a valid cache entry was found. Supports method-specific decompression.
//
// Cache misses, deserialization failures, or invalid entries return false for the found parameter.
func (i *interceptor) getCachedResponse(ctx context.Context, key string, verifier []byte, config *MethodPolicy) (any, error, bool) {
	key = i.buildKey(key)

	var entry cachedEntry
	err := i.cacher.Get(ctx, key, &entry)
	if err != nil {
		// Cache miss or error - don't log ErrMissing as it's expected behavior
		if !errors.Is(err, cache.ErrMissing) {
			i.Logger().Error("failed to get cached entry",
				slog.String("classification", diagnostics.ErrorClass(err)))
		}
		return nil, nil, false //nolint:nilnil // third bool return discriminates cache miss
	}

	if !bytes.Equal(entry.Verifier, verifier) {
		return nil, nil, false //nolint:nilnil // third bool return discriminates cache miss
	}

	if len(entry.Status) != 0 {
		var st statuspb.Status
		if decodeErr := proto.Unmarshal(entry.Status, &st); decodeErr != nil || st.GetCode() == int32(codes.OK) {
			return nil, nil, false //nolint:nilnil // third bool return discriminates cache miss
		}
		return nil, status.FromProto(&st).Err(), true
	}

	// Use method-specific deserialization
	resp, err := i.opts.serializer.UnmarshalForMethod(ctx, entry.Data, config)
	if err != nil {
		// Log deserialization failures - this could indicate cache corruption,
		// version mismatches, or serialization issues
		i.Logger().Error("failed to deserialize cached response",
			slog.String("classification", diagnostics.ErrorClass(err)))
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
	HeaderKey = intern.String("x-cache")
	// HeaderHit is the metadata value indicating a cache hit.
	HeaderHit = intern.String("hit")
	// HeaderMiss is the metadata value indicating a cache miss.
	HeaderMiss = intern.String("miss")
)

// cacheDriver implements the Driver interface for per-call cache operations.
// It handles unary RPC patterns with intelligent caching strategies.
type cacheDriver struct {
	// Immutable fields - set once during creation, safe for concurrent read access
	interceptor *interceptor
	meta        *metadata.CallMetadata
	config      *MethodPolicy

	logger *slog.Logger // Optional logger for debugging and monitoring

	// The key, verifier and req are set in PreCall and read in PostCall. Since
	// cacheDriver is per-call and access is sequential (PreCall → handler →
	// PostCall), no synchronization is needed. A nil verifier means the
	// request bypassed caching.
	key      string
	verifier []byte
	req      any
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

	if d.meta.IsStream {
		return nil, nil //nolint:nilnil
	}

	opts := d.interceptor.opts
	namespace, ok := cacheNamespace(ctx, opts.identityFunc, opts.authRequired)
	if !ok {
		return nil, nil //nolint:nilnil
	}

	method := d.meta.FullyMethodName
	input, err := d.interceptor.discriminators.keyInput(ctx, method, namespace, req)
	if err == nil {
		if opts.keyGenerator != nil {
			d.key, err = opts.keyGenerator(ctx, method, req)
		} else {
			d.key = input.storageKey()
		}
	}
	if err != nil {
		d.logger.Error("failed to generate cache key",
			slog.String("classification", diagnostics.ErrorClass(err)))
		return nil, nil //nolint:nilnil
	}
	d.verifier = input.verifier()
	d.req = req

	// For unary calls, try to get from cache
	cachedResp, cachedErr, found := d.interceptor.getCachedResponse(ctx, d.key, d.verifier, d.config)

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

	if d.verifier == nil {
		return err
	}

	// For cache misses, use decision function to determine if we should cache
	decision := d.interceptor.opts.cacheDecision(ctx, intern.String(d.meta.FullyMethodName), d.req, resp, err)
	if decision.ShouldCache {
		d.interceptor.cacheResponse(ctx, d.key, d.verifier, resp, err, decision.TTL, d.config)
	}

	return err
}
