// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errstatus

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"strconv"

	"github.com/altessa-s/go-atlas/core/collections/slices"
	"github.com/altessa-s/go-atlas/data/cache/lru"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/health"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	stdGrpc "google.golang.org/grpc"
)

var _ interceptors.ServerInterceptor = (*interceptor)(nil)

// cacheEntry stores a cached status along with metadata.
type cacheEntry struct {
	status   *status.Status
	isCustom bool // true if created by a custom converter
}

var defaultSentinelErrors = []error{
	context.DeadlineExceeded,
	context.Canceled,
	health.ErrServiceUnavailable,
}

// StatusError is an interface that allows custom services to convert errors to gRPC status errors.
type StatusError interface {
	StatusErrorConvert(context.Context, error) error
}

type interceptor struct {
	interceptors.BaseInterceptor
	options      *options
	cache        lru.Cacher[string, *cacheEntry]
	allSentinels []error // Pre-built list of all sentinel errors
}

// Dependencies declares that errstatus must run after requestid so that
// DefaultFinalizer can read the request ID from the context.
func (i *interceptor) Dependencies() []string {
	return []string{"requestid"}
}

const convertOperation = "convert"

// ServerInterceptor returns a new interceptor that converts errors to gRPC status errors.
func ServerInterceptor(opt ...Option) interceptors.ServerInterceptor {
	opts := newOptions(opt...)

	// Wrap the finalizer with domain so callers don't need to know about it.
	if opts.finalizer != nil && opts.domain != "" {
		opts.finalizer = withDomainFinalizer(opts.finalizer, opts.domain)
	}

	base := interceptors.NewBaseInterceptor(InterceptorName, opts.logger)

	var cache lru.Cacher[string, *cacheEntry]

	// Only create cache if not disabled and size > 0.
	if !opts.cacheDisabled && opts.cacheSize > 0 {
		var err error
		cache, err = lru.NewCache[string, *cacheEntry](opts.cacheSize)
		if err != nil {
			base.LogError(context.Background(), "failed to create error conversion cache, continue without cache", convertOperation, err,
				slog.Int("size", opts.cacheSize))
			// Continue without cache on error (graceful degradation).
			cache = nil
		}
	}

	// Pre-build the complete list of sentinel errors to avoid repeated allocations.
	allSentinels := make([]error, 0, len(opts.sentinelErrors)+len(defaultSentinelErrors))
	allSentinels = append(allSentinels, opts.sentinelErrors...)
	allSentinels = append(allSentinels, defaultSentinelErrors...)

	return &interceptor{
		BaseInterceptor: base,
		options:         opts,
		cache:           cache,
		allSentinels:    allSentinels,
	}
}

// ServerUnaryInterceptor returns a new unary server interceptor that converts errors to gRPC status errors.
func ServerUnaryInterceptor(opt ...Option) stdGrpc.UnaryServerInterceptor {
	return ServerInterceptor(opt...).ServerUnaryInterceptor()
}

// ServerStreamInterceptor returns a new streaming server interceptor that converts errors to gRPC status errors.
func ServerStreamInterceptor(opt ...Option) stdGrpc.StreamServerInterceptor {
	return ServerInterceptor(opt...).ServerStreamInterceptor()
}

func (i *interceptor) ServerUnaryInterceptor() stdGrpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *stdGrpc.UnaryServerInfo, handler stdGrpc.UnaryHandler) (resp any, err error) {
		defer func() {
			if err != nil && i.options.finalizer != nil {
				err = i.options.finalizer(ctx, err)
			}
		}()

		resp, err = handler(ctx, req)
		if err == nil {
			return resp, nil
		}
		return nil, i.handleServerError(ctx, info.Server, err)
	}
}

func (i *interceptor) ServerStreamInterceptor() stdGrpc.StreamServerInterceptor {
	return func(srv any, stream stdGrpc.ServerStream, info *stdGrpc.StreamServerInfo, handler stdGrpc.StreamHandler) (err error) {
		defer func() {
			if err != nil && i.options.finalizer != nil {
				err = i.options.finalizer(stream.Context(), err)
			}
		}()
		err = handler(srv, stream)
		if err == nil {
			return nil
		}
		return i.handleServerError(stream.Context(), srv, err)
	}
}

// cacheStatus adds a status to the cache if caching is enabled and conditions are met.
func (i *interceptor) cacheStatus(err error, st *status.Status, isCustom bool) {
	if i.cache == nil {
		return
	}
	if i.options.cacheOnlySentinel && !i.isSentinel(err) {
		return
	}
	key := i.makeKey(err)
	i.cache.Put(key, &cacheEntry{status: st, isCustom: isCustom})
}

// logErrorConversion logs error conversion activity for debugging.
func (i *interceptor) logErrorConversion(ctx context.Context, err error, extraAttrs ...slog.Attr) {
	attrs := append([]slog.Attr{
		slog.Any("error", err),
		slog.String("error_type", errorTypeString(err)),
	}, extraAttrs...)
	i.LogDebug(ctx, "handling error conversion", convertOperation, attrs...)
}

// tryCustomConverters attempts to convert an error using custom errorConverters.
// Returns the converted status if a converter matches, nil otherwise.
func (i *interceptor) tryCustomConverters(ctx context.Context, err error) *status.Status {
	for converter := range slices.Values(i.options.errorConverters) {
		if converter.Matcher(ctx, err) {
			i.LogDebug(ctx, "matched custom converter", convertOperation, slog.Any("error", err))
			return converter.Convert(ctx, err)
		}
	}
	return nil
}

// getCachedEntry retrieves a cached error conversion if available.
// Returns the cached entry if found, nil otherwise.
func (i *interceptor) getCachedEntry(ctx context.Context, err error) *cacheEntry {
	if i.cache == nil {
		return nil
	}
	key := i.makeKey(err)
	if entry, found := i.cache.Get(key); found {
		i.LogDebug(ctx, "using cached error conversion", convertOperation,
			slog.Any("error", err),
			slog.Bool("is_custom", entry.isCustom))
		return entry
	}
	return nil
}

// tryBuiltInErrors attempts to convert an error using built-in error type mappings.
// Returns the converted status if a built-in type matches, nil otherwise.
func (i *interceptor) tryBuiltInErrors(ctx context.Context, err error) *status.Status {
	switch {
	case coreerrs.IsContextDeadlineExceeded(err):
		i.LogDebug(ctx, "matched built-in error type: DeadlineExceeded", convertOperation, slog.Any("error", err))
		return status.New(codes.DeadlineExceeded, "Deadline Exceeded")
	case coreerrs.IsContextCanceled(err):
		i.LogDebug(ctx, "matched built-in error type: Canceled", convertOperation, slog.Any("error", err))
		return status.New(codes.Canceled, "Canceled")
	case errors.Is(err, health.ErrServiceUnavailable):
		i.LogDebug(ctx, "matched built-in error type: ServiceUnavailable", convertOperation, slog.Any("error", err))
		return status.New(codes.Unavailable, "Service Unavailable")
	default:
		return nil
	}
}

func (i *interceptor) handleServerError(ctx context.Context, srv any, err error) error {
	i.logErrorConversion(ctx, err)

	// Check if srv is not nil before type assertion.
	if srv != nil {
		if se, ok := srv.(StatusError); ok {
			i.LogDebug(ctx, "using service-level error converter", convertOperation)
			err = se.StatusErrorConvert(ctx, err)
		}
	}

	st, ok := status.FromError(err)
	if !ok {
		// Priority order for error conversion:
		// 1. Cached custom conversions (highest priority - skip converter check)
		// 2. Custom errorConverters (override non-custom cache)
		// 3. Cached non-custom conversions
		// 4. Built-in error types
		// 5. Default Internal error

		// Check cache first.
		entry := i.getCachedEntry(ctx, err)
		if entry != nil && entry.isCustom {
			// Custom cache entry - use it immediately without checking errorConverters again.
			return interceptors.NewError(entry.status, err)
		}

		// Try custom errorConverters (they override non-custom cache).
		if customSt := i.tryCustomConverters(ctx, err); customSt != nil {
			i.cacheStatus(err, customSt, true)
			return interceptors.NewError(customSt, err)
		}

		// Use non-custom cache entry if available.
		if entry != nil {
			return interceptors.NewError(entry.status, err)
		}

		// Try built-in error types.
		if builtInSt := i.tryBuiltInErrors(ctx, err); builtInSt != nil {
			i.cacheStatus(err, builtInSt, false)
			return interceptors.NewError(builtInSt, err)
		}

		// Default to Internal error (not cached as it's not specific).
		i.LogDebug(ctx, "no specific error conversion found, using default Internal error", convertOperation,
			slog.Any("error", err),
			slog.String("error_type", errorTypeString(err)))
		st = status.New(codes.Internal, "Internal Server Error")
		return interceptors.NewError(st, err)
	}

	i.LogDebug(ctx, "error already has gRPC status", convertOperation,
		slog.Any("error", err),
		slog.String("code", st.Code().String()))

	// Handle interceptors.Error wrapping.
	if ie, ok := coreerrs.AsType[*interceptors.Error](err); ok {
		return interceptors.NewError(st, ie.Unwrap())
	}

	return st.Err()
}

// makeKey creates a unique key for an error.
// For sentinel errors (package-level vars), we use pointer address.
// For other errors, we use type + message.
func (i *interceptor) makeKey(err error) string {
	if i.options.cacheOnlySentinel {
		if sentinel := i.resolveSentinel(err); sentinel != nil {
			return sentinelErrorKey(sentinel)
		}
		return "0x" + strconv.FormatUint(uint64(reflect.ValueOf(err).Pointer()), 16)
	}
	// For non-sentinel mode, use type and error message.
	return errorKey(err)
}

// isSentinel checks if an error is a sentinel error (package-level variable).
func (i *interceptor) isSentinel(err error) bool {
	return i.resolveSentinel(err) != nil
}

func (i *interceptor) resolveSentinel(err error) error {
	return findSentinelError(err, i.allSentinels)
}
