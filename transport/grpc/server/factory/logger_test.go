// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/logger"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	grpcconfig "github.com/altessa-s/go-atlas/config/grpc"
	redacttestv1 "github.com/altessa-s/go-atlas/proto/gen/redacttest/v1"
	stdGrpc "google.golang.org/grpc"
)

// providerMethod is the method of the debug-redact test service: it returns its request.
const providerMethod = "/redacttest.v1.Providers/Echo"

func handleProvider(_ any, ctx context.Context, dec func(any) error, ic stdGrpc.UnaryServerInterceptor) (any, error) {
	in := new(redacttestv1.Provider)
	if err := dec(in); err != nil {
		return nil, err
	}
	call := func(_ context.Context, req any) (any, error) { return req, nil }
	if ic == nil {
		return call(ctx, in)
	}
	return ic(ctx, in, &stdGrpc.UnaryServerInfo{FullMethod: providerMethod}, call)
}

// callProvider serves the provider echo service through the builder's chain and sends one Provider carrying apiKey.
func callProvider(t *testing.T, b *ServerBuilder, apiKey string) {
	t.Helper()
	conn := serveService(t, b, &stdGrpc.ServiceDesc{
		ServiceName: "redacttest.v1.Providers",
		HandlerType: (*any)(nil),
		Methods:     []stdGrpc.MethodDesc{{MethodName: "Echo", Handler: handleProvider}},
	})
	var out redacttestv1.Provider
	require.NoError(t, conn.Invoke(t.Context(), providerMethod, &redacttestv1.Provider{Name: "p", ApiKey: proto.String(apiKey)}, &out))
	require.Equal(t, apiKey, out.GetApiKey(), "the redactor must not change the message the handler returns")
}

// TestLoggerInterceptorDebugRedact checks the config flag: on (the default), debug_redact fields are cleared from the logged request
// and response; off, they are logged.
func TestLoggerInterceptorDebugRedact(t *testing.T) {
	t.Parallel()

	const secret = "sk-config-secret"
	defaults := grpcconfig.DefaultLoggerInterceptor()
	require.True(t, defaults.DebugRedact, "debug_redact redaction is on by default")

	for _, tc := range []struct {
		name   string
		redact bool
	}{
		{name: "on", redact: true},
		{name: "off", redact: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			cfg := &grpcconfig.Config{Interceptors: &grpcconfig.Interceptors{Logger: &grpcconfig.LoggerInterceptor{
				BaseInterceptor: enabled, LogRequest: true, LogResponse: true, DebugRedact: tc.redact,
			}}}
			b := New(cfg).
				UseLogger(slog.New(slog.NewJSONHandler(&buf, nil))).
				UseLoggerOptions(logger.WithLogGrpcResponseCodes(uint32(codes.OK))).
				WithLoggerInterceptor()
			callProvider(t, b, secret)

			out := buf.String()
			require.Contains(t, out, `"name":"p"`)
			if tc.redact {
				require.NotContains(t, out, secret)
			} else {
				require.Contains(t, out, secret)
			}
		})
	}
}
