// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiter

import (
	"context"
	"errors"
	"log/slog"

	"github.com/altessa-s/go-atlas/core/text/strings"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/internal/fallback"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sharedlimiter "github.com/altessa-s/go-atlas/data/limiters"
	stdGrpc "google.golang.org/grpc"
)

var _ interceptors.ClientInterceptor = (*clientInterceptor)(nil)

type clientInterceptor struct {
	limiter          sharedlimiter.Limiter
	logger           *slog.Logger
	fallbackBehavior fallback.Behavior
}

// ClientInterceptor returns a new client interceptor that limits the rate of outgoing requests.
// This is useful for implementing client-side rate limiting to avoid overwhelming a server
// or respecting API rate limits.
//
// Example:
//
//	// Create a limiter (e.g., token bucket)
//	limiter := tokenbucket.New(100, 10) // 100 requests/sec, burst of 10
//
//	interceptor := limiter.ClientInterceptor(limiter,
//	    limiter.WithClientLogger(logger),
//	)
//
//	conn, err := grpc.Dial(target,
//	    grpc.WithUnaryInterceptor(interceptor.ClientUnaryInterceptor()),
//	    grpc.WithStreamInterceptor(interceptor.ClientStreamInterceptor()),
//	)
func ClientInterceptor(limiter sharedlimiter.Limiter, opts ...ClientOption) interceptors.ClientInterceptor {
	ic := &clientInterceptor{
		limiter:          limiter,
		logger:           slog.Default(),
		fallbackBehavior: fallback.Deny,
	}

	for _, opt := range opts {
		opt(ic)
	}

	return ic
}

// ClientOption configures the client limiter interceptor.
type ClientOption func(*clientInterceptor)

// WithClientLogger sets the logger for the client interceptor.
func WithClientLogger(logger *slog.Logger) ClientOption {
	return func(c *clientInterceptor) {
		c.logger = logger
	}
}

// WithClientFallbackBehavior sets the fallback behavior when the limiter fails.
func WithClientFallbackBehavior(behavior fallback.Behavior) ClientOption {
	return func(c *clientInterceptor) {
		c.fallbackBehavior = behavior
	}
}

// ClientUnaryInterceptor returns a new unary client interceptor that limits requests.
func ClientUnaryInterceptor(limiter sharedlimiter.Limiter, opts ...ClientOption) stdGrpc.UnaryClientInterceptor {
	return ClientInterceptor(limiter, opts...).ClientUnaryInterceptor()
}

// ClientStreamInterceptor returns a new streaming client interceptor that limits requests.
func ClientStreamInterceptor(limiter sharedlimiter.Limiter, opts ...ClientOption) stdGrpc.StreamClientInterceptor {
	return ClientInterceptor(limiter, opts...).ClientStreamInterceptor()
}

// Name returns the interceptor name.
func (c *clientInterceptor) Name() string {
	return "limiter"
}

// ClientUnaryInterceptor returns a unary client interceptor.
func (c *clientInterceptor) ClientUnaryInterceptor() stdGrpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *stdGrpc.ClientConn,
		invoker stdGrpc.UnaryInvoker,
		opts ...stdGrpc.CallOption,
	) error {
		if err := c.checkLimit(ctx, method); err != nil {
			return err
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// ClientStreamInterceptor returns a streaming client interceptor.
func (c *clientInterceptor) ClientStreamInterceptor() stdGrpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *stdGrpc.StreamDesc,
		cc *stdGrpc.ClientConn,
		method string,
		streamer stdGrpc.Streamer,
		opts ...stdGrpc.CallOption,
	) (stdGrpc.ClientStream, error) {
		if err := c.checkLimit(ctx, method); err != nil {
			return nil, err
		}
		return streamer(ctx, desc, cc, method, opts...)
	}
}

func (c *clientInterceptor) checkLimit(ctx context.Context, method string) error {
	internedMethod := strings.InternString(method)

	info, err := c.limiter.Limit(ctx)
	if err != nil {
		if errors.Is(err, sharedlimiter.ErrLimitExceeded) {
			c.logger.DebugContext(ctx, "client rate limit exceeded",
				slog.String("method", internedMethod))
			return status.New(codes.ResourceExhausted, "Client Rate Limit Exceeded").Err()
		}

		c.logger.ErrorContext(ctx, "client rate limit check failed",
			slog.String("method", internedMethod),
			slog.String("error", err.Error()))

		switch c.fallbackBehavior {
		case fallback.Allow:
			return nil
		case fallback.Deny:
			return status.New(codes.Unavailable, "Rate Limiter Unavailable").Err()
		default:
			return status.New(codes.Internal, "Internal Error").Err()
		}
	}

	if info != nil && info.IsLimitExceeded() {
		c.logger.DebugContext(ctx, "client rate limit exceeded",
			slog.String("method", internedMethod),
			slog.Int64("remaining", info.Remaining))
		return status.New(codes.ResourceExhausted, "Client Rate Limit Exceeded").Err()
	}

	c.logger.DebugContext(ctx, "client rate limit check passed",
		slog.String("method", internedMethod))
	return nil
}
