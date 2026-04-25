// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package logger

import (
	"context"
	"io"
	"time"

	"github.com/altessa-s/go-atlas/core/collections/slices"
	"github.com/altessa-s/go-atlas/core/text/strings"
	"github.com/altessa-s/go-atlas/core/time/timeformat"
	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/driver"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/metadata"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/realip"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/requestid"
	"github.com/altessa-s/go-atlas/transport/internal/clientip"
	"github.com/altessa-s/go-atlas/transport/internal/endpointfilter"
	"github.com/altessa-s/go-atlas/transport/internal/observability"
	"github.com/altessa-s/go-atlas/transport/internal/recovery"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	slogx "github.com/altessa-s/go-atlas/observability/slog"
	tracinginter "github.com/altessa-s/go-atlas/transport/grpc/interceptors/tracing"
	stdSlices "slices"
)

// Logger receives structured call completion events from the interceptor.
// Each invocation represents a finished unary or streaming RPC with the
// resolved gRPC status code and accumulated [slogx.Fields] (method, timing,
// trace IDs, request/response payloads when enabled, etc.).
//
// Use [Slog] for a ready-made implementation that maps gRPC codes to
// [slog.Level] and writes through a standard [slog.Logger].
type Logger interface {
	Log(ctx context.Context, msg string, grpcCode codes.Code, fields slogx.Fields)
}

// LoggerFunc adapts a plain function to the [Logger] interface, allowing
// inline logger definitions without declaring a named type.
type LoggerFunc func(ctx context.Context, msg string, grpcCode codes.Code, fields slogx.Fields)

// Log implements [Logger].
func (f LoggerFunc) Log(ctx context.Context, msg string, grpcCode codes.Code, fields slogx.Fields) {
	f(ctx, msg, grpcCode, fields)
}

var _ driver.DrivenInterceptor = (*interceptor)(nil)
var _ interceptors.Interceptor = (*interceptor)(nil)

type interceptor struct {
	logger        Logger
	opts          *options
	ignoreChecker endpointfilter.Filter
	logCodes      []codes.Code
	logCodesSet   map[codes.Code]struct{} // O(1) lookup for logCodes
}

type requestInterceptor struct {
	*interceptor
	meta *metadata.CallMetadata
}

// ServerInterceptor returns a new interceptor that logs incoming and outgoing gRPC messages.
func ServerInterceptor(logger Logger, opt ...Option) interceptors.ServerInterceptor {
	i := &interceptor{
		logger: logger,
		opts:   newOptions(opt...),
	}

	i.init()

	return interceptors.ServerDrivenInterceptor(i)
}

// ClientInterceptor returns a new interceptor that logs outgoing gRPC client calls.
func ClientInterceptor(logger Logger, opt ...Option) interceptors.ClientInterceptor {
	i := &interceptor{
		logger: logger,
		opts:   newOptions(opt...),
	}

	i.init()

	return interceptors.ClientDrivenInterceptor(i)
}

func (i *interceptor) init() {
	i.ignoreChecker = endpointfilter.NewOrNoop(
		i.opts.ignoreMethods,
		endpointfilter.WithIgnorePatterns(i.opts.ignorePatterns...),
	)

	// Pre-calculate gRPC response codes to log
	if len(i.opts.logGrpcResponseCodes) > 0 {
		i.logCodes = stdSlices.Collect(slices.Map(i.opts.logGrpcResponseCodes, func(c uint32) codes.Code { return codes.Code(c) }))
	} else {
		// Use pre-computed default codes - create a copy to avoid mutation
		i.logCodes = make([]codes.Code, len(DefaultLogCodes))
		copy(i.logCodes, DefaultLogCodes)
	}

	// Remove ignored codes from the log codes
	for _, c := range i.opts.ignoreGrpcResponseCodes {
		i.logCodes = slices.Delete(i.logCodes, codes.Code(c))
	}

	// Pre-compute logCodes set for O(1) lookup
	i.logCodesSet = make(map[codes.Code]struct{}, len(i.logCodes))
	for _, c := range i.logCodes {
		i.logCodesSet[c] = struct{}{}
	}
}

func (i *interceptor) DrivenInterceptor(ctx context.Context) (driver.Driver, context.Context) {
	// Metadata is guaranteed to be in context by the chain
	meta, _ := metadata.FromContext(ctx)

	// Check if this method should be ignored
	if i.ignoreChecker.ShouldFilter(strings.InternLowerString(meta.FullyMethodName)) {
		return driver.NoopDriver(), ctx
	}

	ri := &requestInterceptor{
		interceptor: i,
		meta:        meta,
	}

	// Pre-allocate with expected capacity
	fields := make(slogx.Fields, 0, 15) //nolint:mnd
	fields = append(fields,
		slogx.Field{Key: FieldKeyGrpcMethod, Value: strings.InternString(meta.FullyMethodName)},
		slogx.Field{Key: FieldKeyGrpcStream, Value: meta.IsStream},
		slogx.Field{Key: FieldKeyGrpcService, Value: strings.InternString(meta.ServiceName)},
	)

	fields = slices.AppendIf(fields, !meta.IsClient,
		slogx.Field{Key: observability.FieldKeyClientPeerIP, Value: meta.ClientPerIP},
		slogx.Field{Key: observability.FieldKeyClientUserAgent, Value: meta.ClientUserAgent},
	)

	if d, ok := ctx.Deadline(); ok {
		fields = append(fields, slogx.Field{Key: observability.FieldKeyRequestDeadline, Value: d.Format(time.RFC3339)})
	}

	if realIP := clientip.FromContext(ctx); realIP.IsValid() {
		fields = append(fields, slogx.Field{Key: observability.FieldKeyClientRealIP, Value: realIP})
	}

	if reqID := requestid.FromContext(ctx); reqID != "" {
		fields = append(fields, slogx.Field{Key: observability.FieldKeyRequestID, Value: reqID})
	}

	ctx = slogx.InjectFields(ctx, fields)

	// Inject enriched logger into context if configured
	if i.opts.contextLogger != nil {
		ctx = slogx.InjectLogger(ctx, i.opts.contextLogger)
	}

	return ri, ctx
}

const interceptorName = "logger"

// Name returns the interceptor name used for dependency resolution and chain ordering.
func Name() string { return interceptorName }

// ID is a lightweight [interceptors.Interceptor] reference for this package,
// suitable for passing to exclusion lists.
var ID = interceptors.Ref(interceptorName)

func (i *interceptor) Name() string {
	return interceptorName
}

// Dependencies returns interceptors that logger reads from context.
// All dependencies are optional for ordering - logger gracefully degrades
// if requestid, realip, or tracing data is not available in context.
func (i *interceptor) Dependencies() []string {
	return []string{metadata.Name(), requestid.Name(), realip.Name(), tracinginter.Name()}
}

func (ri *requestInterceptor) PostCall(ctx context.Context, resp any, err error) error {
	if err == io.EOF {
		err = nil
	}

	st := status.Convert(err)
	code := st.Code()

	// Use pre-computed set for O(1) lookup instead of O(n) slice search
	if _, shouldLog := ri.logCodesSet[code]; !shouldLog {
		return err
	}

	fields := slogx.FieldsFromContext(ctx)

	// Batch append for better performance
	now := time.Now()
	fields = append(fields,
		// Use pre-computed interned code string
		slogx.Field{Key: FieldKeyGrpcCode, Value: InternedCodeString(code)},
		slogx.Field{Key: observability.FieldKeyRequestStartTime, Value: timeformat.FormatTime(ri.meta.StartTime, ri.opts.timeFormat)},
		slogx.Field{Key: observability.FieldKeyRequestEndTime, Value: timeformat.FormatTime(now, ri.opts.timeFormat)},
		slogx.Field{Key: observability.FieldKeyRequestDuration, Value: timeformat.FormatDuration(now.Sub(ri.meta.StartTime), ri.opts.timeFormat)},
		slogx.Field{Key: FieldKeyGrpcMessage, Value: st.Message()},
	)

	// Add trace correlation fields if available
	if traceID := tracing.TraceIDFromContext(ctx); traceID != "" {
		fields = append(fields, slogx.Field{Key: observability.FieldKeyTraceID, Value: traceID})
	}
	if spanID := tracing.SpanIDFromContext(ctx); spanID != "" {
		fields = append(fields, slogx.Field{Key: observability.FieldKeySpanID, Value: spanID})
	}

	if st.Code() != codes.OK {
		for detail := range slices.Values(st.Details()) {
			if t, ok := detail.(*errdetails.ErrorInfo); ok {
				fields = append(fields, slogx.Field{Key: FieldKeyGrpcErrorReason, Value: t.GetReason()})
			}
		}

		if perr, ok := coreerrs.AsType[*recovery.PanicError](err); ok {
			fields = append(fields,
				slogx.Field{Key: observability.FieldKeyError, Value: perr.Error()},
				slogx.Field{Key: observability.FieldKeyPanic, Value: perr.Panic},
				slogx.Field{Key: observability.FieldKeyPanicStacktrace, Value: perr.Frames},
			)
		} else if ie, ok := coreerrs.AsType[*interceptors.Error](err); ok {
			fields = append(fields, slogx.Field{Key: observability.FieldKeyError, Value: ie.Error()})
		} else if _, ok := status.FromError(err); !ok {
			fields = append(fields, slogx.Field{Key: observability.FieldKeyError, Value: err.Error()})
		}
	}

	if resp != nil && ri.opts.logResponse {
		if p, ok := resp.(proto.Message); ok {
			fields = append(fields, slogx.Field{Key: observability.FieldKeyResponseContent, Value: p.ProtoReflect().Interface()})
		}
	}

	ri.logger.Log(ctx, "call finished", code, fields)

	return err
}

func (ri *requestInterceptor) PreCall(ctx context.Context, req any) (any, error) {
	if !ri.opts.logRequest {
		return nil, nil //nolint:nilnil
	}

	if !ri.meta.IsClient {
		p, ok := req.(proto.Message)
		if ok {
			slogx.AppendField(ctx, observability.FieldKeyRequestContent, p.ProtoReflect().Interface())
		}
	}

	return nil, nil //nolint:nilnil
}
