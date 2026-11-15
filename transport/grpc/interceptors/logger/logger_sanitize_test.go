// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package logger_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/transport/observability"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/logger"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	slogx "github.com/altessa-s/go-slogx"
)

// captureFields runs one unary call through the server interceptor and
// returns the string fields of its completion record.
func captureFields(t *testing.T, ctx context.Context, method string, callErr error, opts ...logger.Option) map[string]string {
	t.Helper()

	got := map[string]string{}
	sink := logger.LoggerFunc(func(_ context.Context, _ string, _ codes.Code, fields slogx.Fields) {
		for _, f := range fields {
			if s, ok := f.Value.(string); ok {
				got[string(f.Key)] = s
			}
		}
	})

	si := logger.ServerInterceptor(sink, opts...)
	_, err := si.ServerUnaryInterceptor()(ctx, nil, &grpc.UnaryServerInfo{FullMethod: method},
		func(context.Context, any) (any, error) { return nil, callErr })
	require.ErrorIs(t, err, callErr)
	return got
}

func TestServer_CapsPeerControlledFields(t *testing.T) {
	t.Parallel()

	huge := strings.Repeat("a", 1<<20)
	callErr := status.Error(codes.InvalidArgument, "bad name "+huge)

	tests := []struct {
		name                       string
		opts                       []logger.Option
		wantMethod, wantUA, wantMs int
	}{
		{
			name:       "defaults",
			wantMethod: logger.DefaultMaxMethodLength,
			wantUA:     logger.DefaultMaxUserAgentLength,
			wantMs:     logger.DefaultMaxMessageLength,
		},
		{
			name: "configured",
			opts: []logger.Option{
				logger.WithMaxMethodLength(64),
				logger.WithMaxUserAgentLength(32),
				logger.WithMaxMessageLength(16),
			},
			wantMethod: 64,
			wantUA:     32,
			wantMs:     16,
		},
		{
			name: "values below 1 keep the defaults",
			opts: []logger.Option{
				logger.WithMaxMethodLength(0),
				logger.WithMaxUserAgentLength(-1),
				logger.WithMaxMessageLength(0),
			},
			wantMethod: logger.DefaultMaxMethodLength,
			wantUA:     logger.DefaultMaxUserAgentLength,
			wantMs:     logger.DefaultMaxMessageLength,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs("user-agent", huge))
			got := captureFields(t, ctx, "/"+huge+"/Get", callErr, tc.opts...)

			require.Len(t, got[string(logger.FieldKeyGrpcMethod)], tc.wantMethod)
			require.LessOrEqual(t, len(got[string(logger.FieldKeyGrpcService)]), tc.wantMethod)
			require.Len(t, got[string(observability.FieldKeyClientUserAgent)], tc.wantUA)
			require.Len(t, got[string(logger.FieldKeyGrpcMessage)], tc.wantMs)
			for _, key := range []string{
				string(logger.FieldKeyGrpcMethod),
				string(observability.FieldKeyClientUserAgent),
				string(logger.FieldKeyGrpcMessage),
			} {
				require.True(t, strings.HasSuffix(got[key], logger.TruncationSuffix), key)
			}
		})
	}
}

func TestServer_StripsControlCharacters(t *testing.T) {
	t.Parallel()

	ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs("user-agent", "curl/8\r\nlevel=ERROR msg=forged\x7f"))
	got := captureFields(t, ctx, "/example.v1.Service/Get", status.Error(codes.NotFound, "no item \"x\"\nlevel=ERROR"))

	require.Equal(t, "curl/8level=ERROR msg=forged", got[string(observability.FieldKeyClientUserAgent)])
	require.Equal(t, "no item \"x\"level=ERROR", got[string(logger.FieldKeyGrpcMessage)])
	require.Equal(t, "/example.v1.Service/Get", got[string(logger.FieldKeyGrpcMethod)])
	require.Equal(t, "example.v1.Service", got[string(logger.FieldKeyGrpcService)])
}

func TestServer_CapsPlainErrorText(t *testing.T) {
	t.Parallel()

	callErr := errors.New("lookup failed for\n" + strings.Repeat("b", 1<<16))
	got := captureFields(t, t.Context(), "/example.v1.Service/Get", callErr, logger.WithMaxMessageLength(40))

	errText := got[string(observability.FieldKeyError)]
	require.LessOrEqual(t, len(errText), 40)
	require.NotContains(t, errText, "\n")
	require.True(t, strings.HasPrefix(errText, "lookup failed for"))
}
