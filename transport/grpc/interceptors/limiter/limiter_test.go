// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiter

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/coord/limiters"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/defaults"

	"google.golang.org/grpc/metadata"

	stdGrpc "google.golang.org/grpc"
)

type headerCapturingTransportStream struct {
	header metadata.MD
}

func (s *headerCapturingTransportStream) Method() string { return "/svc/Method" }
func (s *headerCapturingTransportStream) SetHeader(md metadata.MD) error {
	s.header = metadata.Join(s.header, md)
	return nil
}
func (s *headerCapturingTransportStream) SendHeader(md metadata.MD) error {
	s.header = metadata.Join(s.header, md)
	return nil
}
func (s *headerCapturingTransportStream) SetTrailer(metadata.MD) error { return nil }

type headerCapturingServerStream struct {
	stdGrpc.ServerStream
	ctx    context.Context
	header metadata.MD
}

func (s *headerCapturingServerStream) Context() context.Context { return s.ctx }
func (s *headerCapturingServerStream) SetHeader(md metadata.MD) error {
	s.header = metadata.Join(s.header, md)
	return nil
}

// TestServerInterceptor_DoesNotEchoOutgoingMetadata verifies that downstream
// outgoing metadata attached to the server context is never echoed back to the
// caller via response headers.
func TestServerInterceptor_DoesNotEchoOutgoingMetadata(t *testing.T) {
	t.Parallel()

	ic := ServerInterceptor(fakeLimiter(), WithExposeHeaders())

	t.Run("unary", func(t *testing.T) {
		t.Parallel()
		ts := &headerCapturingTransportStream{}
		ctx := stdGrpc.NewContextWithServerTransportStream(t.Context(), ts)
		ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer s3cret"))

		_, err := ic.ServerUnaryInterceptor()(ctx, "req", &stdGrpc.UnaryServerInfo{FullMethod: "/svc/Method"}, func(context.Context, any) (any, error) {
			return "ok", nil
		})
		require.NoError(t, err)
		require.Empty(t, ts.header.Get("authorization"))
		require.Equal(t, []string{"100"}, ts.header.Get(RateLimitMetadataKey))
	})

	t.Run("stream", func(t *testing.T) {
		t.Parallel()
		ctx := metadata.NewOutgoingContext(t.Context(), metadata.Pairs("authorization", "Bearer s3cret"))
		ss := &headerCapturingServerStream{ctx: ctx}

		err := ic.ServerStreamInterceptor()(nil, ss, &stdGrpc.StreamServerInfo{FullMethod: "/svc/Method"}, func(any, stdGrpc.ServerStream) error {
			return nil
		})
		require.NoError(t, err)
		require.Empty(t, ss.header.Get("authorization"))
		require.Equal(t, []string{"100"}, ss.header.Get(RateLimitMetadataKey))
	})
}

func fakeLimiter() limiters.Limiter {
	return limiters.Func(func(context.Context) (*limiters.LimitInfo, error) {
		return &limiters.LimitInfo{Limit: 100, Remaining: 42, Reset: 12345}, nil
	})
}

func rateLimitMD(t *testing.T, opts ...Option) metadata.MD {
	t.Helper()
	i := ServerInterceptor(fakeLimiter(), opts...).(*interceptor)
	md, err := i.rateLimit(t.Context(), "/svc/Method")
	require.NoError(t, err)
	return md
}

// TestRateLimitHeaders_OptIn pins the secure default: rate-limit headers reveal
// capacity, so they are withheld unless WithExposeHeaders is set.
func TestRateLimitHeaders_OptIn(t *testing.T) {
	t.Parallel()

	off := rateLimitMD(t)
	require.Empty(t, off.Get(RateLimitMetadataKey), "limit header must be off by default")
	require.Empty(t, off.Get(RateLimitRemaining))
	require.Empty(t, off.Get(RateLimitReset))

	on := rateLimitMD(t, WithExposeHeaders())
	require.Equal(t, []string{"100"}, on.Get(RateLimitMetadataKey))
	require.Equal(t, []string{"42"}, on.Get(RateLimitRemaining))
	require.Equal(t, []string{"12345"}, on.Get(RateLimitReset))
}

// TestDependencies_IncludesAuth verifies the limiter is ordered after auth so a
// present auth interceptor runs first.
func TestDependencies_IncludesAuth(t *testing.T) {
	t.Parallel()

	i := ServerInterceptor(fakeLimiter()).(*interceptor)
	require.Contains(t, i.Dependencies(), auth.Name())
}

// TestRateLimit_NoDefaultExemptions pins that reflection and health consume
// limiter budget like any other method unless the caller opts in to
// defaults.IgnorePatterns.
func TestRateLimit_NoDefaultExemptions(t *testing.T) {
	t.Parallel()

	for _, method := range []string{
		"/grpc.reflection.v1.ServerReflection/ServerReflectionInfo",
		"/grpc.health.v1.Health/Check",
	} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			lim := limiters.Func(func(context.Context) (*limiters.LimitInfo, error) {
				calls.Add(1)
				return &limiters.LimitInfo{Limit: 1, Remaining: 1}, nil
			})

			_, err := ServerInterceptor(lim).(*interceptor).rateLimit(t.Context(), method)
			require.NoError(t, err)
			require.Equal(t, int32(1), calls.Load(), "method must be rate-limited by default")

			optIn := ServerInterceptor(lim, WithIgnorePatterns(defaults.IgnorePatterns...)).(*interceptor)
			_, err = optIn.rateLimit(t.Context(), method)
			require.NoError(t, err)
			require.Equal(t, int32(1), calls.Load(), "opted-in exemption must skip the limiter")
		})
	}
}
