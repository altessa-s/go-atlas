// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errstatus

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=options

import (
	"context"
	"errors"
	"log/slog"

	"github.com/altessa-s/proto-gen-go/io/altessa/badrequest/v1"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/internal/requestid"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	_ "github.com/altessa-s/go-atlas/transport/grpc/interceptors/defaults"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	spb "google.golang.org/genproto/googleapis/rpc/status"
)

// DefaultCacheSize is the default size for error conversion cache.
const DefaultCacheSize = 1000

const interceptorName = "errstatus"

// Name returns the interceptor name used for dependency resolution and chain ordering.
func Name() string { return interceptorName }

// ID is a lightweight [interceptors.Interceptor] reference for this package,
// suitable for passing to exclusion lists.
var ID = interceptors.Ref(interceptorName)

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
	// domain is the service domain name populated in errdetails.ErrorInfo.Domain by the Finalizer
	domain string
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

// DefaultFinalizer returns a [Finalizer] that enriches gRPC status errors
// with additional metadata before they are returned to the client.
//
// The finalizer performs the following operations:
//  1. Adds [errdetails.ErrorInfo] with standardized reason codes (NOT_FOUND, FORBIDDEN, etc.)
//     mapped from gRPC status codes via [GrpcStatusToReasonCode], and sets the Domain field
//     to the value captured from [WithDomain] at interceptor creation time.
//  2. If an [errdetails.ErrorInfo] already exists but has an empty Domain, the configured
//     domain is back-filled.
//  3. Injects [errdetails.RequestInfo] with request ID from context when available
//     (requires requestid interceptor to be configured in the chain).
//  4. Preserves existing error details and [interceptors.Error] wrapping.
//
// Example usage:
//
//	interceptor := errstatus.ServerInterceptor(
//	    errstatus.WithFinalizer(errstatus.DefaultFinalizer),
//	    errstatus.WithDomain("myservice.example.com"),
//	    errstatus.WithErrorMapping(ErrUserNotFound, codes.NotFound, "User not found"),
//	)
//
// DefaultFinalizer is the default [Finalizer] that enriches gRPC status errors
// with [errdetails.ErrorInfo] (reason code) and [errdetails.RequestInfo]
// (request ID from context). Domain is left empty; use
// [DefaultFinalizerWithDomain] to set it.
//
// Example:
//
//	errstatus.ServerInterceptor(
//	    errstatus.WithFinalizer(errstatus.DefaultFinalizer),
//	)
var DefaultFinalizer Finalizer = func(ctx context.Context, err error) error {
	return defaultFinalize(ctx, err, "")
}

// DefaultFinalizerWithDomain returns a [Finalizer] that behaves like
// [DefaultFinalizer] but populates [errdetails.ErrorInfo.Domain] with the
// given domain. If an ErrorInfo already exists with an empty Domain, the
// value is back-filled.
//
// Example:
//
//	errstatus.ServerInterceptor(
//	    errstatus.WithFinalizer(errstatus.DefaultFinalizerWithDomain("myservice.example.com")),
//	)
func DefaultFinalizerWithDomain(domain string) Finalizer {
	return func(ctx context.Context, err error) error {
		return defaultFinalize(ctx, err, domain)
	}
}

// defaultFinalize is the shared implementation for [DefaultFinalizer] and
// [DefaultFinalizerWithDomain].
func defaultFinalize(ctx context.Context, err error, domain string) error {
	st, ok := status.FromError(err)
	if !ok {
		return err
	}

	intercepted, _ := coreerrs.AsType[*interceptors.Error](err)
	if intercepted != nil {
		err = intercepted.Unwrap()
	}

	st = ensureErrorInfo(st, domain)
	st = injectRequestInfo(ctx, st)

	if intercepted != nil {
		return interceptors.NewError(st, err)
	}
	return st.Err()
}

// ensureErrorInfo guarantees the status has an [errdetails.ErrorInfo] detail
// with a reason code and, if provided, a domain. When an ErrorInfo already
// exists but has an empty Domain, the domain is back-filled by rebuilding
// the status cleanly (no in-place proto mutation).
func ensureErrorInfo(st *status.Status, domain string) *status.Status {
	var errorInfo *errdetails.ErrorInfo
	for _, d := range st.Details() {
		if ei, ok := d.(*errdetails.ErrorInfo); ok {
			errorInfo = ei
			break
		}
	}

	if errorInfo == nil {
		ei := &errdetails.ErrorInfo{Reason: reasonForStatus(st), Domain: domain}
		if st2, err := st.WithDetails(ei); err == nil {
			return st2
		}
		return st
	}

	if domain != "" && errorInfo.Domain == "" {
		return rebuildStatusWithDomain(st, domain)
	}

	return st
}

// rebuildStatusWithDomain creates a new status with the domain set on the
// existing [errdetails.ErrorInfo]. It deep-copies the status proto so the
// original is not mutated.
func rebuildStatusWithDomain(st *status.Status, domain string) *status.Status {
	cp, ok := proto.Clone(st.Proto()).(*spb.Status)
	if !ok {
		return st
	}

	for _, detail := range cp.Details {
		if detail.MessageIs(&errdetails.ErrorInfo{}) {
			var ei errdetails.ErrorInfo
			if err := detail.UnmarshalTo(&ei); err == nil {
				ei.Domain = domain
				_ = detail.MarshalFrom(&ei)
			}
			break
		}
	}

	return status.FromProto(cp)
}

// withDomainFinalizer wraps a [Finalizer] so that the configured domain is
// injected into the [defaultFinalize] call. Returns f unchanged if domain
// is empty.
func withDomainFinalizer(f Finalizer, domain string) Finalizer {
	if domain == "" {
		return f
	}
	return func(ctx context.Context, err error) error {
		return defaultFinalize(ctx, err, domain)
	}
}

// reasonForStatus derives the ErrorInfo reason for a status. When the status
// carries a protovalidate [badrequestv1.BadRequest] detail, it promotes the
// primary (first non-empty) FieldViolation.Code — the specific, canonical
// validation reason code — so a client reading ErrorInfo.Reason sees the same
// code as the field violation instead of the generic gRPC-code reason. An empty
// code carries no more information than the gRPC-code reason and is skipped. It
// falls back to [GrpcStatusToReasonCode] when there is no coded field violation.
func reasonForStatus(st *status.Status) string {
	for _, d := range st.Details() {
		br, ok := d.(*badrequestv1.BadRequest)
		if !ok {
			continue
		}
		for _, fv := range br.GetFieldViolations() {
			if code := fv.GetCode(); code != "" {
				return code
			}
		}
	}
	return GrpcStatusToReasonCode(st)
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
