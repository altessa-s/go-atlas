// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/health"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeHealth struct {
	err error
}

func (f fakeHealth) Health(context.Context) error { return f.err }

func okHandler(_ context.Context, _ any) (any, error) { return "ok", nil }

func okStreamHandler(_ any, _ grpc.ServerStream) error { return nil }

type fakeServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (f *fakeServerStream) Context() context.Context { return f.ctx }

func TestName(t *testing.T) {
	t.Parallel()

	require.Equal(t, "health", health.Name())
}

func TestServerUnaryInterceptor_Healthy(t *testing.T) {
	t.Parallel()

	inter := health.ServerUnaryInterceptor(fakeHealth{})
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}

	resp, err := inter(t.Context(), "req", info, okHandler)

	require.NoError(t, err)
	require.Equal(t, "ok", resp)
}

func TestServerUnaryInterceptor_Unhealthy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		checkErr  error
		wantCode  codes.Code
		checkWrap bool
	}{
		{
			name:      "plain error wraps ErrServiceUnavailable",
			checkErr:  errors.New("db down"),
			wantCode:  codes.Unavailable,
			checkWrap: true,
		},
		{
			name:     "grpc status error forwarded as-is",
			checkErr: status.Error(codes.PermissionDenied, "denied"),
			wantCode: codes.PermissionDenied,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			inter := health.ServerUnaryInterceptor(fakeHealth{err: tc.checkErr})
			info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}

			resp, err := inter(t.Context(), "req", info, okHandler)

			require.Nil(t, resp)
			require.Error(t, err)

			st, ok := status.FromError(err)
			require.True(t, ok)
			require.Equal(t, tc.wantCode, st.Code())

			if tc.checkWrap {
				require.ErrorIs(t, err, health.ErrServiceUnavailable)
			}
		})
	}
}

func TestServerStreamInterceptor_Healthy(t *testing.T) {
	t.Parallel()

	inter := health.ServerStreamInterceptor(fakeHealth{})
	info := &grpc.StreamServerInfo{FullMethod: "/test.Service/Stream"}
	stream := &fakeServerStream{ctx: t.Context()}

	called := false
	err := inter(nil, stream, info, func(_ any, got grpc.ServerStream) error {
		called = true
		require.Same(t, stream, got, "a healthy server hands the stream through")
		return nil
	})

	require.NoError(t, err)
	require.True(t, called, "a healthy server runs the handler")
}

func TestServerStreamInterceptor_Unhealthy(t *testing.T) {
	t.Parallel()

	checkErr := errors.New("cache down")
	inter := health.ServerStreamInterceptor(fakeHealth{err: checkErr})
	info := &grpc.StreamServerInfo{FullMethod: "/test.Service/Stream"}
	stream := &fakeServerStream{ctx: t.Context()}

	err := inter(nil, stream, info, okStreamHandler)

	require.Error(t, err)
	require.ErrorIs(t, err, health.ErrServiceUnavailable)

	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.Unavailable, st.Code())
}

// TestServerUnaryInterceptor_WithLogger exercises the WithLogger option and
// confirms the configured logger receives a record for a failed health
// check, which is the observable effect of the logger being wired in.
func TestServerUnaryInterceptor_WithLogger(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	inter := health.ServerUnaryInterceptor(fakeHealth{err: errors.New("db down")}, health.WithLogger(logger))
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}

	_, err := inter(t.Context(), "req", info, okHandler)

	require.Error(t, err)
	require.Contains(t, buf.String(), "health check failed")
}
