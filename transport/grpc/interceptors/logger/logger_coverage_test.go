// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package logger_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/netip"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/time/timeformat"
	"github.com/altessa-s/go-atlas-net/clientip"
	"github.com/altessa-s/go-atlas/internal/transport/observability"
	"github.com/altessa-s/go-atlas/internal/transport/recovery"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/logger"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/metadata"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"

	slogx "github.com/altessa-s/go-slogx"
)

const testMethod = "/svc.Users/Get"

type entry struct {
	code   codes.Code
	fields map[string]any
}

type recorder struct {
	mu      sync.Mutex
	entries []entry
}

func (r *recorder) logger() logger.Logger {
	return logger.LoggerFunc(func(_ context.Context, _ string, code codes.Code, fields slogx.Fields) {
		m := make(map[string]any, len(fields))
		for _, f := range fields {
			m[f.Key] = f.Value
		}
		r.mu.Lock()
		defer r.mu.Unlock()
		r.entries = append(r.entries, entry{code: code, fields: m})
	})
}

func (r *recorder) all() []entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]entry(nil), r.entries...)
}

func runUnary(t *testing.T, si interceptors.ServerInterceptor, ctx context.Context, method string, req any, h grpc.UnaryHandler) (any, error) {
	t.Helper()
	return si.ServerUnaryInterceptor()(ctx, req, &grpc.UnaryServerInfo{FullMethod: method}, h)
}

func TestName(t *testing.T) {
	t.Parallel()

	require.Equal(t, "logger", logger.Name())
	require.Equal(t, "logger", logger.ID.Name())

	si := logger.ServerInterceptor((&recorder{}).logger())
	require.Equal(t, "logger", si.Name())
	deps, ok := si.(interface{ Dependencies() []string })
	require.True(t, ok)
	require.Equal(t, []string{"metadata", "requestid", "realip", "tracing"}, deps.Dependencies())
}

func TestServer_LogsRequestResponseAndContext(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	si := logger.ServerInterceptor(rec.logger(), logger.WithLogRequest(),
		logger.WithLogClientDetails(true), logger.WithLogResponse(),
		logger.WithContextLogger(slog.New(slog.DiscardHandler)))

	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(time.Minute))
	defer cancel()
	ctx = clientip.NewContext(ctx, netip.MustParseAddr("203.0.113.7"))

	var handlerLogger *slog.Logger
	resp, err := runUnary(t, si, ctx, testMethod, wrapperspb.String("request-body"), func(ctx context.Context, _ any) (any, error) {
		handlerLogger = slogx.FromContext(ctx)
		return wrapperspb.String("response-body"), nil
	})
	require.NoError(t, err)
	require.Equal(t, "response-body", resp.(*wrapperspb.StringValue).GetValue())
	require.NotNil(t, handlerLogger)

	entries := rec.all()
	require.Len(t, entries, 1)
	e := entries[0]
	require.Equal(t, codes.OK, e.code)
	require.Equal(t, testMethod, e.fields[logger.FieldKeyGrpcMethod])
	require.Equal(t, "OK", e.fields[logger.FieldKeyGrpcCode])
	require.Equal(t, netip.MustParseAddr("203.0.113.7"), e.fields[observability.FieldKeyClientRealIP])
	require.Contains(t, e.fields, observability.FieldKeyRequestDeadline)
	require.Contains(t, e.fields, observability.FieldKeyRequestDuration)
	require.True(t, proto.Equal(wrapperspb.String("request-body"), e.fields[observability.FieldKeyRequestContent].(proto.Message)))
	require.True(t, proto.Equal(wrapperspb.String("response-body"), e.fields[observability.FieldKeyResponseContent].(proto.Message)))
}

func TestServer_PayloadRedactor(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	si := logger.ServerInterceptor(rec.logger(), logger.WithLogRequest(), logger.WithLogResponse(),
		logger.WithPayloadRedactor(func(proto.Message) any { return "[redacted]" }))

	_, err := runUnary(t, si, t.Context(), testMethod, wrapperspb.String("secret"), func(context.Context, any) (any, error) {
		return wrapperspb.String("secret"), nil
	})
	require.NoError(t, err)
	e := rec.all()[0]
	require.Equal(t, "[redacted]", e.fields[observability.FieldKeyRequestContent])
	require.Equal(t, "[redacted]", e.fields[observability.FieldKeyResponseContent])
}

func TestServer_ErrorFields(t *testing.T) {
	t.Parallel()

	withReason, err := status.New(codes.PermissionDenied, "nope").WithDetails(&errdetails.ErrorInfo{Reason: "TENANT_LOCKED"})
	require.NoError(t, err)

	tests := []struct {
		name       string
		err        error
		wantCode   codes.Code
		wantReason string
		wantError  string
		wantPanic  bool
	}{
		{name: "status with error info", err: withReason.Err(), wantCode: codes.PermissionDenied, wantReason: "TENANT_LOCKED"},
		{name: "plain error", err: errors.New("boom"), wantCode: codes.Unknown, wantError: "boom"},
		{
			name:      "interceptor error",
			err:       interceptors.NewError(status.New(codes.Unavailable, "down"), errors.New("db unreachable")),
			wantCode:  codes.Unavailable,
			wantError: "db unreachable",
		},
		{
			name:      "panic error",
			err:       recovery.NewPanicError("kaboom", 0),
			wantCode:  codes.Unknown,
			wantError: recovery.NewPanicError("kaboom", 0).Error(),
			wantPanic: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := &recorder{}
			si := logger.ServerInterceptor(rec.logger(), logger.WithLogResponse(), logger.WithLogRawErrors(true))
			_, err := runUnary(t, si, t.Context(), testMethod, nil, func(context.Context, any) (any, error) { return nil, tc.err })
			require.ErrorIs(t, err, tc.err)

			entries := rec.all()
			require.Len(t, entries, 1)
			e := entries[0]
			require.Equal(t, tc.wantCode, e.code)
			if tc.wantReason != "" {
				require.Equal(t, tc.wantReason, e.fields[logger.FieldKeyGrpcErrorReason])
			}
			if tc.wantError != "" {
				require.Equal(t, tc.wantError, e.fields[observability.FieldKeyError])
			} else {
				require.NotContains(t, e.fields, observability.FieldKeyError)
			}
			if tc.wantPanic {
				require.Equal(t, "kaboom", e.fields[observability.FieldKeyPanic])
				require.Contains(t, e.fields, observability.FieldKeyPanicStacktrace)
			}
		})
	}
}

func TestServer_CodeFiltering(t *testing.T) {
	t.Parallel()

	notFound := func(context.Context, any) (any, error) { return nil, status.Error(codes.NotFound, "x") }
	ok := func(context.Context, any) (any, error) { return wrapperspb.String("ok"), nil }

	tests := []struct {
		name    string
		opts    []logger.Option
		handler grpc.UnaryHandler
		logged  bool
	}{
		{name: "default logs not found", handler: notFound, logged: true},
		{name: "ignored code", opts: []logger.Option{logger.WithIgnoreGrpcResponseCodes(uint32(codes.NotFound))}, handler: notFound},
		{name: "explicit log codes exclude ok", opts: []logger.Option{logger.WithLogGrpcResponseCodes(uint32(codes.NotFound))}, handler: ok},
		{name: "explicit log codes include not found", opts: []logger.Option{logger.WithLogGrpcResponseCodes(uint32(codes.NotFound))}, handler: notFound, logged: true},
		{name: "ignored method", opts: []logger.Option{logger.WithIgnoreMethods(testMethod)}, handler: ok},
		{name: "ignored pattern folds case", opts: []logger.Option{logger.WithIgnorePatterns(regexp.MustCompile(`^/svc\.users/get$`))}, handler: ok},
		{name: "mixed-case pattern needs (?i)", opts: []logger.Option{logger.WithIgnorePatterns(regexp.MustCompile(`(?i)^/svc\.Users/`))}, handler: ok},
		{name: "mixed-case pattern sees lowercased method", opts: []logger.Option{logger.WithIgnorePatterns(regexp.MustCompile(`^/svc\.Users/`))}, handler: ok, logged: true},
		{name: "empty options are no-ops", opts: []logger.Option{
			logger.WithIgnoreMethods(), logger.WithIgnorePatterns(), logger.WithLogGrpcResponseCodes(), logger.WithIgnoreGrpcResponseCodes(), logger.WithContextLogger(nil),
		}, handler: ok, logged: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := &recorder{}
			_, _ = runUnary(t, logger.ServerInterceptor(rec.logger(), tc.opts...), t.Context(), testMethod, nil, tc.handler)
			require.Equal(t, tc.logged, len(rec.all()) == 1)
		})
	}
}

func TestServer_HandlerEOF(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	_, err := runUnary(t, logger.ServerInterceptor(rec.logger()), t.Context(), testMethod, nil,
		func(context.Context, any) (any, error) { return nil, io.EOF })
	require.ErrorIs(t, err, io.EOF, "the handler error is returned unchanged")
	require.Equal(t, codes.Unknown, rec.all()[0].code, "a handler returning EOF sends Unknown on the wire")
}

func TestServer_TimeFormat(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	l := logger.Slog(slog.New(slog.NewJSONHandler(&buf, nil)))
	si := logger.ServerInterceptor(l, logger.WithTimeFormat(""))
	_, err := runUnary(t, si, t.Context(), testMethod, nil, func(context.Context, any) (any, error) { return wrapperspb.String("ok"), nil })
	require.NoError(t, err)
	require.Contains(t, buf.String(), `"grpc.code":"OK"`)
	require.Contains(t, buf.String(), `"msg":"call finished"`)
}

func TestClientInterceptor(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	ci := logger.ClientInterceptor(rec.logger(), logger.WithLogRequest(), logger.WithLogResponse())
	unary := ci.ClientUnaryInterceptor()

	reply := wrapperspb.String("")
	err := unary(t.Context(), testMethod, wrapperspb.String("req"), reply, nil,
		func(_ context.Context, _ string, _, reply any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
			reply.(*wrapperspb.StringValue).Value = "resp"
			return nil
		})
	require.NoError(t, err)

	entries := rec.all()
	require.Len(t, entries, 1)
	e := entries[0]
	require.Equal(t, codes.OK, e.code)
	require.Equal(t, testMethod, e.fields[logger.FieldKeyGrpcMethod])
	require.NotContains(t, e.fields, observability.FieldKeyRequestContent, "client side does not log the request payload")
	require.NotContains(t, e.fields, observability.FieldKeyClientPeerIP, "client side omits peer fields")
	require.True(t, proto.Equal(wrapperspb.String("resp"), e.fields[observability.FieldKeyResponseContent].(proto.Message)))

	err = unary(t.Context(), testMethod, wrapperspb.String("req"), wrapperspb.String(""), nil,
		func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
			return status.Error(codes.Unavailable, "down")
		})
	require.Equal(t, codes.Unavailable, status.Code(err))
	require.Len(t, rec.all(), 2)
}

func TestInternedCodeString(t *testing.T) {
	t.Parallel()
	require.Equal(t, "NotFound", logger.InternedCodeString(codes.NotFound))
	require.Equal(t, codes.Code(99).String(), logger.InternedCodeString(codes.Code(99)))
}

func TestClientCallTimes(t *testing.T) {
	t.Parallel()
	rec := &recorder{}
	ic := logger.ClientInterceptor(rec.logger(), logger.WithTimeFormat(timeformat.UnixNano))
	// A proxy can call the same method using its incoming server context.
	ctx := metadata.NewContext(t.Context(), &metadata.CallMetadata{FullyMethodName: testMethod, StartTime: time.Now().Add(-time.Hour)})
	before := time.Now()
	var entered, returned time.Time
	err := ic.ClientUnaryInterceptor()(ctx, testMethod, nil, nil, nil, func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
		entered = time.Now()
		time.Sleep(5 * time.Millisecond)
		returned = time.Now()
		return nil
	})
	after := time.Now()
	require.NoError(t, err)
	e := rec.all()[0]
	start := time.Unix(0, e.fields[observability.FieldKeyRequestStartTime].(int64))
	end := time.Unix(0, e.fields[observability.FieldKeyRequestEndTime].(int64))
	duration := time.Duration(e.fields[observability.FieldKeyRequestDuration].(int64))
	require.False(t, start.Before(before))
	require.False(t, start.After(entered))
	require.False(t, end.Before(returned))
	require.False(t, end.After(after))
	require.GreaterOrEqual(t, duration, returned.Sub(entered))
	require.LessOrEqual(t, duration, after.Sub(before))
}
