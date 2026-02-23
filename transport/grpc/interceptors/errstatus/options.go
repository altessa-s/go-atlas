// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errstatus

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate --type=options

import (
	"context"
	"errors"
	"log/slog"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/internal/requestid"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	_ "github.com/altessa-s/go-atlas/transport/grpc/interceptors/defaults"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// DefaultCacheSize is the default size for error conversion cache.
const DefaultCacheSize = 1000

// InterceptorName is the name of this interceptor.
const InterceptorName = "errstatus"

// ErrorConverter defines a custom error conversion strategy for transforming
// application errors into gRPC status errors. It consists of a matcher function
// that determines if the converter should handle a specific error, and a convert
// function that performs the actual transformation.
//
// Error errorConverters are evaluated in the order they are registered, with the first
// matching converter being used. Custom errorConverters take precedence over built-in
// error conversions, allowing override of default behavior.
type ErrorConverter struct {
	// Matcher determines whether this converter should handle the given error
	Matcher func(context.Context, error) bool
	// Convert transforms the matched error into a gRPC status
	Convert func(context.Context, error) *status.Status
}

// StatusConverter defines a custom status-to-error conversion strategy for
// transforming gRPC status errors back into application errors on the client side.
// It consists of a matcher function that determines if the converter should handle
// a specific status code, and a convert function that performs the transformation.
//
// Status errorConverters are evaluated in the order they are registered, with the first
// matching converter being used. This allows clients to convert standard gRPC status
// codes back into domain-specific errors.
type StatusConverter struct {
	// Matcher determines whether this converter should handle the given status
	Matcher func(context.Context, *status.Status) bool
	// Convert transforms the matched status into an application error
	Convert func(context.Context, *status.Status) error
}

// Finalizer performs post-processing on errors after they have been converted
// to gRPC status errors. Finalizers are executed using a defer pattern,
// ensuring they run even if panics occur during error conversion.
//
// The finalizer receives the request context and the final error (after conversion),
// and returns a potentially modified error.
type Finalizer func(ctx context.Context, err error) error

// statusConverterIndex holds indexed status errorConverters for fast lookup.
// Converters are split into two groups:
// - Fast path: errorConverters that match by gRPC code only (O(1) lookup)
// - Slow path: errorConverters with complex matchers that need Details inspection
type statusConverterIndex struct {
	// fastPath maps gRPC codes to errorConverters that only check the code
	// This enables O(1) lookup for simple code-based conversions
	fastPath map[codes.Code][]StatusConverter
	// slowPath contains errorConverters with complex matchers (check details, etc.)
	// These are evaluated sequentially in registration order
	slowPath []StatusConverter
}

// options holds the configuration for the error status interceptor.
// These options are set through functional option functions and control
// how errors are converted, cached, and processed.
type options struct {
	// errorConverters holds the list of custom error errorConverters in priority order
	errorConverters []ErrorConverter `opt:"-"`
	// statusConverter holds the list of custom status-to-error errorConverters for client-side
	statusConverter []StatusConverter `opt:"-"`
	// statusConverterIndex holds indexed status errorConverters for O(1) fast path lookup
	statusConverterIndex *statusConverterIndex `opt:"-"`
	// sentinelErrors holds additional sentinel errors eligible for caching in sentinel-only mode
	sentinelErrors []error
	// logger is used for debugging error conversion behavior
	logger *slog.Logger
	// finalizer is the post-processing function for converted errors
	finalizer Finalizer
	// cacheSize is the maximum number of entries in the conversion cache
	cacheSize int `optgen:"default=DefaultCacheSize"`
	// cacheDisabled completely disables error conversion caching
	cacheDisabled bool
	// cacheOnlySentinel restricts caching to sentinel errors only to prevent memory leaks
	cacheOnlySentinel bool
}

// WithErrorConverters adds multiple custom error errorConverters in a single call.
// Converters are appended to the existing list and evaluated in the order provided.
func WithErrorConverters(converters ...ErrorConverter) Option {
	return func(o *options) {
		o.errorConverters = append(o.errorConverters, converters...)
	}
}

// WithErrorMapping adds a simple error mapping converter for common use cases.
// It creates an ErrorConverter using errors.Is() for matching and converts
// the error to the specified gRPC code and message.
//
// If message is empty, uses err.Error() as the status message.
func WithErrorMapping(target error, code codes.Code, message string) Option {
	return WithErrorConverters(ErrorConverter{
		Matcher: func(ctx context.Context, err error) bool {
			return errors.Is(err, target)
		},
		Convert: func(ctx context.Context, err error) *status.Status {
			msg := message
			if msg == "" {
				msg = err.Error()
			}
			return status.New(code, msg)
		},
	})
}

// WithErrorTypeMapping adds a type-safe converter for custom error types using Go generics.
// It creates an ErrorConverter that uses errors.As() to match errors of type T
// and extracts custom messages using the provided getMessage function.
//
// If getMessage returns an empty string, err.Error() will be used as fallback.
func WithErrorTypeMapping[T error](code codes.Code, getMessage func(T) string) Option {
	return WithErrorConverters(ErrorConverter{
		Matcher: func(ctx context.Context, err error) bool {
			_, ok := coreerrs.AsType[T](err)
			return ok
		},
		Convert: func(ctx context.Context, err error) *status.Status {
			if target, ok := coreerrs.AsType[T](err); ok {
				msg := getMessage(target)
				if msg == "" {
					msg = err.Error()
				}
				return status.New(code, msg)
			}
			return status.New(codes.Internal, "Internal Server Error")
		},
	})
}

// WithStatusConverters adds client-side [StatusConverter] entries that map
// incoming gRPC statuses back to application errors. Converters are appended
// to the existing list and evaluated in registration order; the first match wins.
func WithStatusConverters(converters ...StatusConverter) Option {
	return func(o *options) {
		o.statusConverter = append(o.statusConverter, converters...)
	}
}

// WithStatusMapping adds a simple status-to-error mapping converter for common use cases.
// It creates a StatusConverter that matches the specified gRPC code and converts it
// to the target error.
func WithStatusMapping(code codes.Code, targetErr error) Option {
	return WithStatusConverters(StatusConverter{
		Matcher: func(ctx context.Context, st *status.Status) bool {
			return st.Code() == code
		},
		Convert: func(ctx context.Context, st *status.Status) error {
			return targetErr
		},
	})
}

// WithStatusConverterFunc adds a simple status-to-error converter using a function.
// It creates a StatusConverter that matches the specified gRPC code and converts it
// to the target error.
func WithStatusConverterFunc(c func(context.Context, *status.Status) error) Option {
	return WithStatusConverters(StatusConverter{
		Matcher: func(ctx context.Context, st *status.Status) bool { return true },
		Convert: c,
	})
}

// DefaultFinalizer is the default finalizer function that enriches gRPC status errors
// with additional metadata before they are returned to the client.
//
// The finalizer performs the following operations:
//  1. Adds ErrorInfo details with standardized reason codes (NOT_FOUND, FORBIDDEN, etc.)
//     mapped from gRPC status codes via GrpcStatusToReasonCode function.
//  2. Injects RequestInfo details with request ID from context when available
//     (requires requestid interceptor to be configured in the chain).
//  3. Preserves existing error details and interceptors.Error wrapping.
//
// This finalizer is useful for providing consistent error metadata across all services
// and improving error observability in distributed systems. It can be configured using
// WithFinalizer option, or replaced with a custom finalizer for application-specific needs.
//
// Example usage:
//
//	interceptor := errstatus.ServerInterceptor(
//	    errstatus.WithFinalizer(errstatus.DefaultFinalizer),
//	    errstatus.WithErrorMapping(ErrUserNotFound, codes.NotFound, "User not found"),
//	)
func DefaultFinalizer(ctx context.Context, err error) error {
	st, ok := status.FromError(err)
	if !ok {
		return err
	}

	intercepted, _ := coreerrs.AsType[*interceptors.Error](err)
	if intercepted != nil {
		err = intercepted.Unwrap()
	}

	var errorInfo *errdetails.ErrorInfo
	for _, d := range st.Details() {
		if errorInfo, ok = d.(*errdetails.ErrorInfo); ok {
			break
		}
	}

	if errorInfo == nil {
		errorInfo = &errdetails.ErrorInfo{Reason: GrpcStatusToReasonCode(st)}
		if st2, errDetails := st.WithDetails(errorInfo); errDetails == nil {
			st = st2
		}
		// Note: If WithDetails fails, we continue with the original status.
		// This is acceptable as the finalizer should not fail the entire operation
		// due to metadata enrichment issues.
	}

	st = injectRequestInfo(ctx, st)
	if intercepted != nil {
		return interceptors.NewError(st, err)
	}
	return st.Err()
}

// GrpcStatusToReasonCode maps gRPC status codes to error reason codes.
// This function is used to set the reason in the ErrorInfo detail.
func GrpcStatusToReasonCode(st *status.Status) string {
	reason := "UNKNOWN"
	switch st.Code() {
	case codes.NotFound:
		reason = "NOT_FOUND"
	case codes.PermissionDenied:
		reason = "FORBIDDEN"
	case codes.InvalidArgument:
		reason = "INVALID_ARGUMENT"
	case codes.Unavailable:
		reason = "TEMPORARY_UNAVAILABLE"
	case codes.Internal:
		reason = "INTERNAL_ERROR"
	case codes.Unimplemented:
		reason = "UNIMPLEMENTED"
	case codes.Aborted:
		reason = "ABORTED"
	case codes.Unauthenticated:
		reason = "UNAUTHENTICATED"
	case codes.FailedPrecondition:
		reason = "FAILED_PRECONDITION"
	case codes.DeadlineExceeded:
		reason = "DEADLINE_EXCEEDED"
	case codes.ResourceExhausted:
		reason = "RESOURCE_EXHAUSTED"
	case codes.Canceled:
		reason = "CANCELED"
	default:
	}
	return reason
}

// injectRequestInfo adds request information to the error status if not already present.
// This is useful for tracing and debugging purposes, especially in distributed systems.
func injectRequestInfo(ctx context.Context, st *status.Status) *status.Status {
	if reqId := requestid.FromContext(ctx); reqId != "" {
		hasRequestInfo := false
		for _, d := range st.Details() {
			if _, ok := d.(*errdetails.RequestInfo); ok {
				hasRequestInfo = true
			}
		}

		if !hasRequestInfo {
			if st2, errDetails := st.WithDetails(&errdetails.RequestInfo{RequestId: reqId}); errDetails == nil {
				st = st2
			}
			// Note: If WithDetails fails, we continue with the original status.
			// Request ID injection is best-effort and should not fail the operation.
		}
	}
	return st
}
