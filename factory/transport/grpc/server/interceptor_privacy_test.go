// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/transport/grpc/server"
	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/errstatus"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldmask"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	grpcconfig "github.com/altessa-s/go-atlas/config/grpc"
	grpcserver "github.com/altessa-s/go-atlas/transport/grpc/server"
	grpc_testing "google.golang.org/grpc/interop/grpc_testing"
)

func TestFieldMaskReadExtractorPanic(t *testing.T) {
	t.Parallel()

	const marker = "extractor-panic-secret"
	var logs bytes.Buffer
	cfg := &grpcconfig.Config{Interceptors: &grpcconfig.Interceptors{
		ErrStatus: &grpcconfig.ErrStatusInterceptor{Enabled: true},
		Recovery:  &grpcconfig.RecoveryInterceptor{BaseInterceptor: on},
		FieldMask: &grpcconfig.FieldMaskInterceptor{BaseInterceptor: on},
	}}
	b := factory.New(cfg).UseLogger(slog.New(slog.NewTextHandler(&logs, nil))).
		UseFieldMaskOptions(fieldmask.WithMethodKind(covMethod, fieldmask.KindRead),
			fieldmask.WithReadExtractor(covMethod, func(_ context.Context, req proto.Message) (*fieldmaskpb.FieldMask, bool) {
				if req.(*wrapperspb.StringValue).GetValue() == "panic" {
					panic(marker)
				}
				return nil, false
			}))
	conn, svc := startBuilt(t, b.WithInterceptors())

	_, err := echo(t.Context(), conn, "panic")
	st := status.Convert(err)
	require.Equal(t, codes.Internal, st.Code())
	require.NotContains(t, st.Proto().String(), marker)
	require.Contains(t, logs.String(), "panic_recovered")
	require.NotContains(t, logs.String(), marker)
	require.NotContains(t, logs.String(), "stack_trace")
	require.Zero(t, svc.calls.Load())

	got, err := echo(t.Context(), conn, "still serving")
	require.NoError(t, err)
	require.Equal(t, "still serving", got)
	require.Equal(t, int32(1), svc.calls.Load())
}

func TestInvalidReadMaskRedactsRejectedPath(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	cfg := &grpcconfig.Config{Interceptors: &grpcconfig.Interceptors{
		ErrStatus: &grpcconfig.ErrStatusInterceptor{Enabled: true},
		FieldMask: &grpcconfig.FieldMaskInterceptor{BaseInterceptor: on},
		Logger:    &grpcconfig.LoggerInterceptor{BaseInterceptor: on, LogRawErrors: true},
	}}
	b := factory.New(cfg).UseLogger(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))).
		UseFieldMaskOptions(fieldmask.WithMethodKind(covMethod, fieldmask.KindRead))
	conn, _ := startBuilt(t, b.WithInterceptors())

	for _, mask := range []string{"secret_field", "*,secret_field"} {
		ctx := metadata.AppendToOutgoingContext(t.Context(), "x-goog-fieldmask", mask)
		_, err := echo(ctx, conn, "echo")
		st := status.Convert(err)
		require.Equal(t, codes.InvalidArgument, st.Code())
		require.NotEmpty(t, st.Details())
		info, ok := st.Details()[0].(*errdetails.ErrorInfo)
		require.True(t, ok)
		require.Equal(t, "INVALID_FIELD_MASK", info.GetReason())
		require.NotContains(t, st.Proto().String(), "secret_field")
	}
	require.Contains(t, logs.String(), "INVALID_FIELD_MASK")
	require.NotContains(t, logs.String(), "secret_field")
}

func TestTCPInterceptorPrivacyDefaults(t *testing.T) {
	t.Parallel()

	for _, format := range []struct {
		name    string
		handler func(io.Writer, *slog.HandlerOptions) slog.Handler
	}{
		{"text", func(w io.Writer, opts *slog.HandlerOptions) slog.Handler { return slog.NewTextHandler(w, opts) }},
		{"json", func(w io.Writer, opts *slog.HandlerOptions) slog.Handler { return slog.NewJSONHandler(w, opts) }},
	} {
		t.Run(format.name, func(t *testing.T) {
			t.Parallel()

			const (
				userAgent  = "stage-b-private-user-agent"
				forwarded  = "8.8.4.4"
				malformed  = "forwarded-metadata-secret"
				internalIP = "10.12.13.14"
			)
			cfg := &grpcconfig.Config{Interceptors: &grpcconfig.Interceptors{
				RealIp:    &grpcconfig.RealIPInterceptor{BaseInterceptor: on, TrustedPeers: []string{"127.0.0.1/32"}},
				Logger:    &grpcconfig.LoggerInterceptor{BaseInterceptor: on, EnableContextLogger: true},
				Tracing:   &grpcconfig.TracingInterceptor{BaseInterceptor: on},
				ErrStatus: &grpcconfig.ErrStatusInterceptor{Enabled: true},
			}}
			lis, err := net.Listen("tcp4", "127.0.0.1:0")
			require.NoError(t, err)
			t.Cleanup(func() { _ = lis.Close() })

			var logs bytes.Buffer
			log := slog.New(format.handler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
			capture := statusCapture{adapters.NewMultiAdapter(), make(chan adapters.SpanData, 2)}
			tr := tracing.New(tracing.WithAdapter(capture))
			t.Cleanup(func() { require.NoError(t, tr.Shutdown(context.WithoutCancel(t.Context()))) })

			b := factory.New(cfg).UseLogger(log).UseTracer(tr).
				UseErrStatusOptions(errstatus.WithLogger(log)).
				UseServerOptions(grpcserver.WithListener(lis), grpcserver.WithStartupVerification(0)).
				WithInterceptors()
			srv, err := b.Build()
			require.NoError(t, err)
			srv.RegisterHandlers(&statusService{fail: func() error {
				return &net.OpError{Op: "read", Net: "tcp", Addr: &net.TCPAddr{IP: net.ParseIP(internalIP), Port: 5432}, Err: io.ErrUnexpectedEOF}
			}})
			require.NoError(t, srv.Start())
			shutdown := func() {
				ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 5*time.Second)
				defer cancel()
				require.NoError(t, srv.Shutdown(ctx))
			}
			t.Cleanup(shutdown)

			conn, err := grpc.NewClient("passthrough:///"+lis.Addr().String(),
				grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithUserAgent(userAgent))
			require.NoError(t, err)
			t.Cleanup(func() { _ = conn.Close() })
			client := grpc_testing.NewTestServiceClient(conn)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()

			_, err = client.EmptyCall(metadata.AppendToOutgoingContext(ctx, "x-forwarded-for", forwarded), &grpc_testing.Empty{})
			require.Equal(t, codes.Internal, status.Code(err))
			rpc, err := client.StreamingOutputCall(metadata.AppendToOutgoingContext(ctx, "x-forwarded-for", malformed), &grpc_testing.StreamingOutputCallRequest{})
			require.NoError(t, err)
			_, err = rpc.Recv()
			require.Equal(t, codes.Internal, status.Code(err))

			shutdown()
			require.NoError(t, tr.ForceFlush(ctx))
			require.Len(t, capture.spans, 2)
			gotLogs := logs.String()
			require.Contains(t, gotLogs, "call finished")
			require.Contains(t, gotLogs, "grpc.code")
			// Startup messages legitimately include the server's listen address.
			gotLogs = string(bytes.ReplaceAll([]byte(gotLogs), []byte(lis.Addr().String()), nil))
			for _, marker := range []string{"127.0.0.1", "client.peer_ip", "client.real_ip", "client.user_agent", forwarded, malformed, userAgent, internalIP, "read tcp"} {
				require.NotContains(t, gotLogs, marker)
			}
			for range 2 {
				span := <-capture.spans
				require.Equal(t, adapters.StatusError, span.Status)
				require.Contains(t, span.Attributes, adapters.Attribute{Key: "rpc.grpc.status_code", Value: int(codes.Internal)})
				require.Empty(t, span.StatusDesc)
				require.Empty(t, span.Events)
				for _, marker := range []string{"127.0.0.1", "net.peer.name", forwarded, userAgent, internalIP, "read tcp"} {
					require.NotContains(t, fmt.Sprint(span.Attributes), marker)
				}
			}
		})
	}
}
