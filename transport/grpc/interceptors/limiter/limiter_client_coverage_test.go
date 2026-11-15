// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiter

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/coord/limiters"
	"github.com/altessa-s/go-atlas/transport/policy/fallback"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func clientLimiter(info *limiters.LimitInfo, err error) limiters.Limiter {
	return limiters.Func(func(context.Context) (*limiters.LimitInfo, error) {
		return info, err
	})
}

func okInvoker(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
	return nil
}

func TestClientInterceptor_Name(t *testing.T) {
	t.Parallel()

	i := ClientInterceptor(clientLimiter(&limiters.LimitInfo{Remaining: 1}, nil))
	require.Equal(t, "limiter", i.Name())
}

func TestClientUnaryInterceptor_Allowed(t *testing.T) {
	t.Parallel()

	called := false
	invoker := func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
		called = true
		return nil
	}

	inter := ClientUnaryInterceptor(clientLimiter(&limiters.LimitInfo{Limit: 10, Remaining: 9}, nil))
	err := inter(t.Context(), "/svc/Method", "req", "reply", nil, invoker)

	require.NoError(t, err)
	require.True(t, called, "invoker should be called when under the limit")
}

func TestClientUnaryInterceptor_LimitExceededError(t *testing.T) {
	t.Parallel()

	called := false
	invoker := func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
		called = true
		return nil
	}

	inter := ClientUnaryInterceptor(clientLimiter(nil, limiters.ErrLimitExceeded))
	err := inter(t.Context(), "/svc/Method", "req", "reply", nil, invoker)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.ResourceExhausted, st.Code())
	require.False(t, called, "invoker must not run once the limit is exceeded")
}

func TestClientUnaryInterceptor_InfoReportsExceeded(t *testing.T) {
	t.Parallel()

	called := false
	invoker := func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
		called = true
		return nil
	}

	inter := ClientUnaryInterceptor(clientLimiter(&limiters.LimitInfo{Limit: 10, Remaining: 0}, nil))
	err := inter(t.Context(), "/svc/Method", "req", "reply", nil, invoker)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.ResourceExhausted, st.Code())
	require.False(t, called)
}

func TestClientUnaryInterceptor_LimiterFailure_FallbackBehavior(t *testing.T) {
	t.Parallel()

	limiterErr := errors.New("backend down")

	tests := []struct {
		name       string
		behavior   fallback.Behavior
		wantCalled bool
		wantCode   codes.Code
	}{
		{"default deny", "", false, codes.Unavailable},
		{"explicit deny", fallback.Deny, false, codes.Unavailable},
		{"allow", fallback.Allow, true, codes.OK},
		{"unknown falls to internal", fallback.Error, false, codes.Internal},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			called := false
			invoker := func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
				called = true
				return nil
			}

			var opts []ClientOption
			if tc.behavior != "" {
				opts = append(opts, WithClientFallbackBehavior(tc.behavior))
			}

			inter := ClientUnaryInterceptor(clientLimiter(nil, limiterErr), opts...)
			err := inter(t.Context(), "/svc/Method", "req", "reply", nil, invoker)

			require.Equal(t, tc.wantCalled, called)
			if tc.wantCode == codes.OK {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			st, ok := status.FromError(err)
			require.True(t, ok)
			require.Equal(t, tc.wantCode, st.Code())
		})
	}
}

func TestClientStreamInterceptor_Allowed(t *testing.T) {
	t.Parallel()

	called := false
	streamer := func(context.Context, *grpc.StreamDesc, *grpc.ClientConn, string, ...grpc.CallOption) (grpc.ClientStream, error) {
		called = true
		return nil, nil
	}

	inter := ClientStreamInterceptor(clientLimiter(&limiters.LimitInfo{Limit: 10, Remaining: 9}, nil))
	stream, err := inter(t.Context(), &grpc.StreamDesc{}, nil, "/svc/Stream", streamer)

	require.NoError(t, err)
	require.Nil(t, stream)
	require.True(t, called)
}

func TestClientStreamInterceptor_LimitExceeded(t *testing.T) {
	t.Parallel()

	called := false
	streamer := func(context.Context, *grpc.StreamDesc, *grpc.ClientConn, string, ...grpc.CallOption) (grpc.ClientStream, error) {
		called = true
		return nil, nil
	}

	inter := ClientStreamInterceptor(clientLimiter(nil, limiters.ErrLimitExceeded))
	stream, err := inter(t.Context(), &grpc.StreamDesc{}, nil, "/svc/Stream", streamer)

	require.Nil(t, stream)
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.ResourceExhausted, st.Code())
	require.False(t, called)
}

func TestClientInterceptor_WithClientLogger(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	inter := ClientUnaryInterceptor(clientLimiter(nil, limiters.ErrLimitExceeded), WithClientLogger(logger))
	err := inter(t.Context(), "/svc/Method", "req", "reply", nil, okInvoker)

	require.Error(t, err)
	require.Contains(t, buf.String(), "client rate limit exceeded")
}

func TestClientInterceptor_PassesThroughMetadata(t *testing.T) {
	t.Parallel()

	// Under the limit, both interceptors hand the caller's context (with its
	// outgoing metadata), method and arguments to the next handler unchanged.
	ctx := metadata.AppendToOutgoingContext(t.Context(), "x-trace", "abc")
	under := func() limiters.Limiter { return clientLimiter(&limiters.LimitInfo{Limit: 1, Remaining: 1}, nil) }
	assertMD := func(t *testing.T, got context.Context) {
		t.Helper()
		md, ok := metadata.FromOutgoingContext(got)
		require.True(t, ok)
		require.Equal(t, []string{"abc"}, md.Get("x-trace"))
	}

	var unaryCalled bool
	invoker := func(got context.Context, method string, req, reply any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		unaryCalled = true
		assertMD(t, got)
		require.Equal(t, "/svc/Method", method)
		require.Equal(t, "req", req)
		require.Equal(t, "reply", reply)
		return nil
	}
	require.NoError(t, ClientUnaryInterceptor(under())(ctx, "/svc/Method", "req", "reply", nil, invoker))
	require.True(t, unaryCalled, "the invoker runs under the limit")

	desc := &grpc.StreamDesc{StreamName: "Stream"}
	var streamCalled bool
	streamer := func(got context.Context, gotDesc *grpc.StreamDesc, _ *grpc.ClientConn, method string,
		_ ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		streamCalled = true
		assertMD(t, got)
		require.Same(t, desc, gotDesc)
		require.Equal(t, "/svc/Stream", method)
		return nil, nil
	}
	_, err := ClientStreamInterceptor(under())(ctx, desc, nil, "/svc/Stream", streamer)
	require.NoError(t, err)
	require.True(t, streamCalled, "the streamer runs under the limit")
}
