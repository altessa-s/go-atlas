// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errstatus

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync/atomic"

	"github.com/altessa-s/proto-gen-go/io/altessa/badrequest/v1"

	"github.com/altessa-s/go-atlas/internal/diagnostics"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/health"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/internal/callstatus"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/logger"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/metrics"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/requestid"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/tracing"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/anypb"

	coremaps "github.com/altessa-s/go-atlas-core/collections/maps"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	stdGrpc "google.golang.org/grpc"
)

var _ interceptors.ServerInterceptor = (*interceptor)(nil)

// Statuses of the built-in conversions. They do not depend on the error, so
// one value serves every request.
var (
	statusDeadlineExceeded = status.New(codes.DeadlineExceeded, "Deadline Exceeded")
	statusCanceled         = status.New(codes.Canceled, "Canceled")
	statusUnavailable      = status.New(codes.Unavailable, "Service Unavailable")
	statusInternal         = status.New(codes.Internal, MessageInternal)
)

// StatusError is an interface that allows custom services to convert errors to gRPC status errors.
type StatusError interface {
	StatusErrorConvert(context.Context, error) error
}

type interceptor struct {
	interceptors.BaseInterceptor
	options *options
	// cache holds the status of each [ErrorConverter.Static] converter,
	// indexed by the converter's position in options.errorConverters; it
	// covers the first cacheSize converters and is nil when caching is
	// disabled. The key is the converter that matched, never a property of
	// the error, so an entry is the same for every error the converter
	// matches. Which converter matches is decided for every error by running
	// the matchers in order.
	cache []atomic.Pointer[status.Status]
}

// debugEnabled reports whether debug records are logged. Hot paths check it
// before building log attributes, which would otherwise be allocated only to
// be dropped.
func (i *interceptor) debugEnabled(ctx context.Context) bool {
	return i.options.logger != nil && i.options.logger.Enabled(ctx, slog.LevelDebug)
}

// Dependencies declares that errstatus must run after requestid so that
// DefaultFinalizer can read the request ID from the context, and after
// the observers so that they see the finalized status.
func (i *interceptor) Dependencies() []string {
	return []string{requestid.Name(), tracing.Name(), logger.Name(), metrics.Name()}
}

func errorAttr(err error) slog.Attr {
	return slog.Any("error", diagnostics.LogError(err))
}

const convertOperation = "convert"

// MessageInternal is the client-facing message of every codes.Internal status
// the interceptor produces; the original error text stays in the server logs.
const MessageInternal = "Internal Server Error"

// ServerInterceptor returns a new interceptor that converts errors to gRPC status errors.
func ServerInterceptor(opt ...Option) interceptors.ServerInterceptor {
	opts := newOptions(opt...)

	// Wrap the finalizer with domain so callers don't need to know about it.
	if opts.finalizer != nil && opts.domain != "" {
		opts.finalizer = withDomainFinalizer(opts.finalizer, opts.domain)
	}

	base := interceptors.NewBaseInterceptor(interceptorName, opts.logger)

	var cache []atomic.Pointer[status.Status]

	// Only create cache if not disabled and size > 0.
	if !opts.cacheDisabled && opts.cacheSize > 0 {
		cache = make([]atomic.Pointer[status.Status], min(len(opts.errorConverters), opts.cacheSize))
	}

	return &interceptor{
		BaseInterceptor: base,
		options:         opts,
		cache:           cache,
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
		if err = i.handleServerError(ctx, info.Server, err); err == nil {
			// The service-level converter suppressed the error.
			return resp, nil
		}
		return nil, err
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

// tryCustomConverters converts err with the first custom converter whose
// matcher accepts it, or returns nil. The matchers run for every error, in
// registration order, so an error that several converters match always gets
// the first one. Only the status of a [ErrorConverter.Static] converter is
// cached, under the converter itself.
func (i *interceptor) tryCustomConverters(ctx context.Context, err error) *status.Status {
	for idx, converter := range i.options.errorConverters {
		if !converter.Matcher(ctx, err) {
			continue
		}
		if i.debugEnabled(ctx) {
			i.LogDebug(ctx, "matched custom converter", convertOperation, errorAttr(err))
		}
		cached := converter.Static && idx < len(i.cache)
		if cached {
			if st := i.cache[idx].Load(); st != nil {
				return st
			}
		}
		st := i.sanitizeConverted(ctx, converter.Convert(ctx, err), err)
		if cached && st != nil {
			i.cache[idx].Store(st)
		}
		return st
	}
	return nil
}

// sanitizeConverted applies to the status of a converter the rules of a
// status a handler returns directly, so a mapping cannot publish what the
// status path would withhold: a server-fault status gets the fixed message
// and the server-fault details of [clientStatus], and any other status loses
// its errdetails.DebugInfo. It runs before the status is cached.
func (i *interceptor) sanitizeConverted(ctx context.Context, st *status.Status, err error) *status.Status {
	if st == nil {
		return nil
	}
	if !isServerFault(st.Code()) {
		out, stripped := withoutDebugInfo(st)
		if stripped {
			i.logDroppedDetails(ctx, st.Code(), []string{string(debugInfoName)})
		}
		return out
	}
	if st.Message() == serverFaultMessage(st.Code()) && len(st.Details()) == 0 {
		return st
	}
	out, dropped := clientStatus(st, err)
	if out.Message() != st.Message() {
		i.LogWarn(ctx, "gRPC status message replaced for the client", convertOperation, err,
			slog.String("code", st.Code().String()),
			slog.Bool("converter", true))
	}
	i.logDroppedDetails(ctx, st.Code(), dropped)
	return out
}

// logDroppedDetails logs the type names of the details left out of a status.
func (i *interceptor) logDroppedDetails(ctx context.Context, code codes.Code, dropped []string) {
	if len(dropped) == 0 {
		return
	}
	i.LogDebug(ctx, "gRPC status details dropped", convertOperation,
		slog.String("code", code.String()),
		slog.Any("dropped", dropped))
}

// tryBuiltInErrors attempts to convert an error using built-in error type mappings.
// Returns the converted status if a built-in type matches, nil otherwise.
func (i *interceptor) tryBuiltInErrors(ctx context.Context, err error) *status.Status {
	var st *status.Status
	switch {
	case coreerrs.IsContextDeadlineExceeded(err):
		st = statusDeadlineExceeded
	case coreerrs.IsContextCanceled(err):
		st = statusCanceled
	case errors.Is(err, health.ErrServiceUnavailable):
		st = statusUnavailable
	default:
		return nil
	}
	if i.debugEnabled(ctx) {
		i.LogDebug(ctx, "matched built-in error type", convertOperation,
			errorAttr(err), slog.String("code", st.Code().String()))
	}
	return st
}

// convertError converts an error that carries no gRPC status. Priority order:
//  1. Custom errorConverters, first match in registration order
//  2. Built-in error types
//  3. Default Internal error
func (i *interceptor) convertError(ctx context.Context, err error, debug bool) error {
	if customSt := i.tryCustomConverters(ctx, err); customSt != nil {
		return interceptors.NewError(customSt, err)
	}
	if builtInSt := i.tryBuiltInErrors(ctx, err); builtInSt != nil {
		return interceptors.NewError(builtInSt, err)
	}
	if debug {
		i.LogDebug(ctx, "no specific error conversion found, using default Internal error", convertOperation,
			errorAttr(err),
			slog.String("error_type", errorTypeString(err)))
	}
	return interceptors.NewError(statusInternal, err)
}

func (i *interceptor) handleServerError(ctx context.Context, srv any, err error) error {
	callstatus.Save(ctx, err)
	debug := i.debugEnabled(ctx)
	if debug {
		i.LogDebug(ctx, "handling error conversion", convertOperation,
			errorAttr(err),
			slog.String("error_type", errorTypeString(err)))
	}

	// Check if srv is not nil before type assertion.
	if srv != nil {
		if se, ok := srv.(StatusError); ok {
			i.LogDebug(ctx, "using service-level error converter", convertOperation)
			err = se.StatusErrorConvert(ctx, err)
		}
	}

	// A service-level converter suppresses an error by returning nil.
	if err == nil {
		return nil
	}

	// status.FromError builds an Unknown status out of an error that carries
	// none; checking for a carrier first keeps that off the common path.
	if _, carries := errors.AsType[grpcStatuser](err); !carries {
		return i.convertError(ctx, err, debug)
	}
	st, ok := status.FromError(err)
	if !ok {
		return i.convertError(ctx, err, debug)
	}
	// An OK status is not an error either; a non-nil error carrying it would
	// reach the client as Unknown.
	if st.Code() == codes.OK {
		return nil
	}

	if debug {
		i.LogDebug(ctx, "error already has gRPC status", convertOperation,
			errorAttr(err),
			slog.String("code", st.Code().String()))
	}

	// Only a status the handler returned directly is sent as is. For a
	// status found through wrapping, status.FromError reports the whole
	// err.Error() chain as the message, and an Internal, Unknown or DataLoss
	// status describes a server fault; both would publish internals.
	_, direct := err.(grpcStatuser)
	if !direct || isServerFault(st.Code()) {
		i.LogWarn(ctx, "gRPC status message replaced for the client", convertOperation, err,
			slog.String("code", st.Code().String()),
			slog.Bool("wrapped", !direct))
		clientSt, dropped := clientStatus(st, err)
		i.logDroppedDetails(ctx, st.Code(), dropped)
		cause := err
		if ie, ok := errors.AsType[*interceptors.Error](err); ok && direct {
			cause = ie.Unwrap()
		}
		return interceptors.NewError(clientSt, cause)
	}

	// A direct client-facing status keeps its message and details, except
	// errdetails.DebugInfo, which no path forwards.
	st, stripped := withoutDebugInfo(st)
	if stripped {
		i.logDroppedDetails(ctx, st.Code(), []string{string(debugInfoName)})
	}

	// Handle interceptors.Error wrapping.
	if ie, ok := errors.AsType[*interceptors.Error](err); ok {
		return interceptors.NewError(st, ie.Unwrap())
	}
	if stripped {
		return interceptors.NewError(st, err)
	}

	return st.Err()
}

// debugInfoName is the protobuf name of errdetails.DebugInfo.
var debugInfoName = detailName(&errdetails.DebugInfo{})

// withoutDebugInfo returns st without its errdetails.DebugInfo details and
// reports whether it removed any. A status that carries none is returned as
// is, without copying it. st.Details is the only accessor that does not clone
// the status, so it gates the scan of the type URLs; a detail it cannot decode
// is scanned too.
func withoutDebugInfo(st *status.Status) (*status.Status, bool) {
	suspect := slices.ContainsFunc(st.Details(), func(d any) bool {
		switch d.(type) {
		case *errdetails.DebugInfo, error:
			return true
		default:
			return false
		}
	})
	if !suspect {
		return st, false
	}
	p := st.Proto() // a copy; the original status is not mutated
	n := len(p.Details)
	p.Details = slices.DeleteFunc(p.Details, func(d *anypb.Any) bool { return d.MessageName() == debugInfoName })
	if len(p.Details) == n {
		return st, false
	}
	return status.FromProto(p), true
}

// isServerFault reports whether code describes a failure inside the server,
// whose message is not meant for clients.
func isServerFault(code codes.Code) bool {
	return code == codes.Internal || code == codes.Unknown || code == codes.DataLoss
}

// serverFaultMessage returns the fixed client-facing message of a server-fault
// status: [MessageInternal] for codes.Internal, the code's name otherwise.
func serverFaultMessage(code codes.Code) string {
	if code == codes.Internal {
		return MessageInternal
	}
	return code.String()
}

// grpcStatuser is the interface of errors that carry a gRPC status.
type grpcStatuser interface {
	error
	GRPCStatus() *status.Status
}

// detailName returns the full protobuf name of a status detail type.
func detailName(m proto.Message) protoreflect.FullName {
	return m.ProtoReflect().Descriptor().FullName()
}

// clientSafeDetails is the allowlist of status detail types that
// [clientStatus] forwards. The value reports whether the type is also
// forwarded on a server-fault status ([isServerFault]); the rest describe the
// client's own request and only make sense on a client-facing code.
// errdetails.DebugInfo is absent on purpose: it is never forwarded.
var clientSafeDetails = coremaps.NewImmutableMap(map[protoreflect.FullName]bool{
	detailName(&errdetails.RetryInfo{}):           true,
	detailName(&errdetails.RequestInfo{}):         true,
	detailName(&errdetails.LocalizedMessage{}):    true,
	detailName(&errdetails.Help{}):                true,
	detailName(&errdetails.BadRequest{}):          false,
	detailName(&badrequestv1.BadRequest{}):        false,
	detailName(&errdetails.PreconditionFailure{}): false,
	detailName(&errdetails.QuotaFailure{}):        false,
	detailName(&errdetails.ResourceInfo{}):        false,
	detailName(&errdetails.ErrorInfo{}):           false,
})

// clientStatus returns the status sent to the client in place of st, the
// status of err, when st is a server fault or was found through wrapping. It
// also returns the type names of the details it left out.
//
// Two policies apply, chosen by the code:
//   - server fault (Internal, Unknown, DataLoss): the message is fixed,
//     [MessageInternal] for codes.Internal and the code's name otherwise, and
//     only the [clientSafeDetails] marked for server faults are kept;
//   - any other code, reached only for a wrapped status: the message is the
//     wrapped status's own, without the wrapper text, and every detail in
//     [clientSafeDetails] is kept.
func clientStatus(st *status.Status, err error) (*status.Status, []string) {
	serverFault := isServerFault(st.Code())

	p := st.Proto() // a copy; the original status is not mutated
	if serverFault {
		p.Message = serverFaultMessage(st.Code())
	} else {
		p.Message = st.Code().String()
		if inner, ok := errors.AsType[grpcStatuser](err); ok && inner.GRPCStatus() != nil {
			p.Message = inner.GRPCStatus().Message()
		}
	}

	var dropped []string
	p.Details = slices.DeleteFunc(p.Details, func(d *anypb.Any) bool {
		onServerFault, safe := clientSafeDetails.Get(d.MessageName())
		if safe && (onServerFault || !serverFault) {
			return false
		}
		dropped = append(dropped, string(d.MessageName()))
		return true
	})
	return status.FromProto(p), dropped
}
