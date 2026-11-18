// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiter

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-net/httpx"
	"github.com/altessa-s/go-atlas/coord/limiters"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/realip"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type fakeServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (f *fakeServerStream) Context() context.Context { return f.ctx }

func (f *fakeServerStream) SetHeader(metadata.MD) error { return nil }

func serverLimiter(info *limiters.LimitInfo, err error) limiters.Limiter {
	return limiters.Func(func(context.Context) (*limiters.LimitInfo, error) {
		return info, err
	})
}

func TestName_And_RequiredDependencies(t *testing.T) {
	t.Parallel()

	require.Equal(t, "limiter", Name())

	i := ServerInterceptor(fakeLimiter()).(*interceptor) //nolint:errcheck
	require.Equal(t, []string{realip.Name()}, i.RequiredDependencies())
}

func TestServerUnaryInterceptor_Allowed(t *testing.T) {
	t.Parallel()

	called := false
	handler := func(ctx context.Context, _ any) (any, error) {
		called = true
		return "ok", nil
	}

	inter := ServerUnaryInterceptor(serverLimiter(&limiters.LimitInfo{Limit: 10, Remaining: 9}, nil))
	info := &grpc.UnaryServerInfo{FullMethod: "/svc/Method"}

	resp, err := inter(t.Context(), "req", info, handler)

	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, "ok", resp)
}

func TestServerUnaryInterceptor_LimitExceeded(t *testing.T) {
	t.Parallel()

	called := false
	handler := func(context.Context, any) (any, error) {
		called = true
		return "ok", nil
	}

	inter := ServerUnaryInterceptor(serverLimiter(nil, limiters.ErrLimitExceeded))
	info := &grpc.UnaryServerInfo{FullMethod: "/svc/Method"}

	resp, err := inter(t.Context(), "req", info, handler)

	require.Nil(t, resp)
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.ResourceExhausted, st.Code())
	require.False(t, called)
}

func TestServerUnaryInterceptor_LimiterFailure_FallbackDeny(t *testing.T) {
	t.Parallel()

	inter := ServerUnaryInterceptor(serverLimiter(nil, errors.New("backend down")))
	info := &grpc.UnaryServerInfo{FullMethod: "/svc/Method"}

	resp, err := inter(t.Context(), "req", info, func(context.Context, any) (any, error) {
		return "ok", nil
	})

	require.Nil(t, resp)
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.Unavailable, st.Code())
}

func TestServerStreamInterceptor_Allowed(t *testing.T) {
	t.Parallel()

	called := false
	handler := func(any, grpc.ServerStream) error {
		called = true
		return nil
	}

	inter := ServerStreamInterceptor(serverLimiter(&limiters.LimitInfo{Limit: 10, Remaining: 9}, nil))
	info := &grpc.StreamServerInfo{FullMethod: "/svc/Stream"}

	err := inter(nil, &fakeServerStream{ctx: t.Context()}, info, handler)

	require.NoError(t, err)
	require.True(t, called)
}

func TestServerStreamInterceptor_LimitExceeded(t *testing.T) {
	t.Parallel()

	called := false
	handler := func(any, grpc.ServerStream) error {
		called = true
		return nil
	}

	inter := ServerStreamInterceptor(serverLimiter(nil, limiters.ErrLimitExceeded))
	info := &grpc.StreamServerInfo{FullMethod: "/svc/Stream"}

	err := inter(nil, &fakeServerStream{ctx: t.Context()}, info, handler)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.ResourceExhausted, st.Code())
	require.False(t, called)
}

func TestExtractBearerToken(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		headers []string
		want    string
	}{
		{"valid", []string{"Bearer abc.def=="}, "abc.def=="},
		{"lowercase", []string{"bearer xyz"}, "xyz"},
		{"missing", nil, ""},
		{"empty", []string{""}, ""},
		{"wrong scheme", []string{"Basic abc"}, ""},
		{"two spaces", []string{"Bearer  abc"}, ""},
		{"invalid character", []string{"Bearer a:b"}, ""},
		{"duplicate", []string{"Bearer abc", "Bearer abc"}, ""},
		{"too long", []string{"Bearer " + strings.Repeat("a", httpx.MaxBearerLength+1)}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := metadata.NewIncomingContext(t.Context(), metadata.MD{"authorization": tc.headers})
			require.Equal(t, tc.want, extractBearerToken(ctx))
		})
	}
}
