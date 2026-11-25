// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/time/timeformat"
	"github.com/altessa-s/go-atlas/factory/transport/grpc/server"
	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"
	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/errstatus"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	grpcconfig "github.com/altessa-s/go-atlas/config/grpc"
	grpc_testing "google.golang.org/grpc/interop/grpc_testing"
)

type statusService struct {
	grpc_testing.UnimplementedTestServiceServer
	fail func() error
}

func (s *statusService) Register(gs grpc.ServiceRegistrar, _ <-chan struct{}) {
	grpc_testing.RegisterTestServiceServer(gs, s)
}

func (s *statusService) EmptyCall(context.Context, *grpc_testing.Empty) (*grpc_testing.Empty, error) {
	return nil, s.fail()
}

func (s *statusService) StreamingOutputCall(_ *grpc_testing.StreamingOutputCallRequest, _ grpc_testing.TestService_StreamingOutputCallServer) error {
	return s.fail()
}

var errAlreadyAccepted = errors.New("request already accepted")

func (*statusService) StatusErrorConvert(_ context.Context, err error) error {
	if errors.Is(err, errAlreadyAccepted) {
		return status.Error(codes.FailedPrecondition, "already accepted")
	}
	return err
}

type statusCapture struct {
	*adapters.MultiAdapter
	spans chan adapters.SpanData
}

func (a statusCapture) ExportSpans(_ context.Context, spans []adapters.SpanData) error {
	for _, s := range spans {
		a.spans <- s
	}
	return nil
}

func TestFactoryFinalStatus(t *testing.T) {
	const raw = "query postgres://name:password@db/atlas failed"
	const safe = "query postgres://db/atlas failed"
	const id = "550e8400-e29b-41d4-a716-446655440000"

	for _, tc := range []struct {
		name, rawType, cause, message string
		code                          codes.Code
		spanStatus                    adapters.StatusCode
	}{
		{"mapped", "*errors.errorString", errAlreadyAccepted.Error(), "already accepted", codes.FailedPrecondition, adapters.StatusUnset},
		{"unmapped", "*errors.errorString", safe, errstatus.MessageInternal, codes.Internal, adapters.StatusError},
		{"panic", "*recovery.PanicError", "panic: dial postgres://db failed", errstatus.MessageInternal, codes.Internal, adapters.StatusError},
		{"auth", "*diagnostics.classifiedError", "Unauthenticated", "Unauthenticated", codes.Unauthenticated, adapters.StatusUnset},
		{"request ID", "*status.Error", status.Error(codes.InvalidArgument, "invalid request ID, it must be a UUID v4").Error(),
			"invalid request ID, it must be a UUID v4", codes.InvalidArgument, adapters.StatusUnset},
		{"finalizer", "*errors.errorString", safe, "try again", codes.Unavailable, adapters.StatusError},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, stream), func(t *testing.T) {
				t.Parallel()
				cfg := &grpcconfig.Config{Interceptors: &grpcconfig.Interceptors{
					ErrStatus: &grpcconfig.ErrStatusInterceptor{BaseInterceptor: on},
					Auth:      &grpcconfig.AuthInterceptor{BaseInterceptor: on},
					RequestId: &grpcconfig.RequestIDInterceptor{BaseInterceptor: on, HeaderName: "x-request-id"},
					Tracing:   &grpcconfig.TracingInterceptor{BaseInterceptor: on, RecordRawErrors: true},
					Logger:    &grpcconfig.LoggerInterceptor{BaseInterceptor: on, LogRawErrors: true, TimeFormat: timeformat.RFC3339Nano},
					Metrics:   &grpcconfig.MetricsInterceptor{BaseInterceptor: on, Subsystem: "rpc"},
					Recovery:  &grpcconfig.RecoveryInterceptor{BaseInterceptor: on, LogRawErrors: true},
				}}
				capture := statusCapture{adapters.NewMultiAdapter(), make(chan adapters.SpanData, 1)}
				tr := tracing.New(tracing.WithAdapter(capture))
				t.Cleanup(func() { require.NoError(t, tr.Shutdown(context.WithoutCancel(t.Context()))) })
				var logs bytes.Buffer
				meter := metricstest.NewTestCollector()
				b := factory.New(cfg).UseMeter(meter).UseTracer(tr).UseLogger(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))).
					UseAuth(auth.AuthFunc(func(context.Context, auth.Request) (any, error) {
						if tc.name == "auth" {
							return nil, errors.New("auth at postgres://name:password@db/atlas failed")
						}
						return "user", nil
					}), nil)
				if tc.name == "finalizer" {
					b.UseErrStatusOptions(errstatus.WithFinalizer(func(ctx context.Context, _ error) error {
						return errstatus.DefaultFinalizer(ctx, status.Error(codes.Unavailable, "try again"))
					}))
				}
				svc := &statusService{fail: func() error {
					switch tc.name {
					case "mapped":
						return errAlreadyAccepted
					case "panic":
						panic("dial postgres://name:password@db failed")
					default:
						return errors.New(raw)
					}
				}}
				client := grpc_testing.NewTestServiceClient(startBuiltService(t, b.WithInterceptors(), svc))
				ctx := metadata.NewOutgoingContext(t.Context(), metadata.Pairs("authorization", "Bearer test", "x-request-id", id))
				if tc.name == "request ID" {
					ctx = t.Context()
				}
				method := grpc_testing.TestService_EmptyCall_FullMethodName
				before := time.Now()
				var err error
				if stream {
					method = grpc_testing.TestService_StreamingOutputCall_FullMethodName
					rpc, openErr := client.StreamingOutputCall(ctx, &grpc_testing.StreamingOutputCallRequest{})
					require.NoError(t, openErr)
					_, err = rpc.Recv()
				} else {
					_, err = client.EmptyCall(ctx, &grpc_testing.Empty{})
				}
				after := time.Now()
				wire := status.Convert(err)
				require.Equal(t, tc.code, wire.Code())
				require.Equal(t, tc.message, wire.Message())
				require.NotContains(t, wire.Proto().String(), "password")
				if tc.name == "request ID" {
					require.Len(t, wire.Details(), 1)
				} else {
					require.Len(t, wire.Details(), 2)
					require.Equal(t, id, wire.Details()[1].(*errdetails.RequestInfo).RequestId)
				}
				require.Equal(t, 1.0, metricstest.GetCounterValue(t, meter, "test_rpc_server_requests_total", "method", method, "status", tc.code.String()))
				require.NoError(t, tr.ForceFlush(ctx))
				require.Len(t, capture.spans, 1)
				span := <-capture.spans
				require.Equal(t, tc.spanStatus, span.Status)
				if tc.spanStatus == adapters.StatusError {
					require.Equal(t, tc.message, span.StatusDesc)
				}
				require.Contains(t, span.Attributes, adapters.Attribute{Key: "rpc.grpc.status_code", Value: int(tc.code)})
				require.Len(t, span.Events, 1)
				require.Equal(t, "exception", span.Events[0].Name)
				require.ElementsMatch(t, []adapters.Attribute{{Key: "exception.type", Value: tc.rawType}, {Key: "exception.message", Value: tc.cause}}, span.Events[0].Attributes)
				require.NotContains(t, logs.String(), "password")
				var records int
				for line := range strings.SplitSeq(strings.TrimSpace(logs.String()), "\n") {
					var record struct {
						Msg, Error string
						Code       string        `json:"grpc.code"`
						Message    string        `json:"grpc.message"`
						Start      time.Time     `json:"request.start_time"`
						End        time.Time     `json:"request.end_time"`
						Duration   time.Duration `json:"request.duration"`
					}
					require.NoError(t, json.Unmarshal([]byte(line), &record))
					if record.Msg != "call finished" {
						continue
					}
					records++
					require.Equal(t, tc.code.String(), record.Code)
					require.Equal(t, tc.message, record.Message)
					require.Equal(t, tc.cause, record.Error)
					require.False(t, record.Start.Before(before))
					require.False(t, record.End.After(after))
					require.Positive(t, record.Duration)
					require.LessOrEqual(t, record.Duration, after.Sub(before))
				}
				require.Equal(t, 1, records)
			})
		}
	}
}
