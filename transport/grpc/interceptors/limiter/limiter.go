// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiter

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"

	"github.com/altessa-s/go-atlas/data/limiters/tokenbucket"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/internal/clientip"
	"github.com/altessa-s/go-atlas/transport/internal/fallback"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	internstrings "github.com/altessa-s/go-atlas/core/text/strings"
	sharedlimiter "github.com/altessa-s/go-atlas/data/limiters"
	stdGrpc "google.golang.org/grpc"
)

// Interned gRPC metadata keys set by [ServerInterceptor], [ServerUnaryInterceptor],
// and [ServerStreamInterceptor] on every rate-limited response. Clients can read
// these headers from the response metadata to implement adaptive backoff.
var (
	// RateLimitMetadataKey is the metadata key for the total request limit.
	RateLimitMetadataKey = internstrings.InternString("x-ratelimit-limit")
	// RateLimitRemaining is the metadata key for remaining requests in the current window.
	RateLimitRemaining = internstrings.InternString("x-ratelimit-remaining")
	// RateLimitReset is the metadata key for the Unix timestamp when the limit resets.
	RateLimitReset = internstrings.InternString("x-ratelimit-reset")
)

// Ensure interceptor implements the ServerInterceptor interface.
var _ interceptors.ServerInterceptor = (*interceptor)(nil)

type interceptor struct {
	interceptors.BaseInterceptor
	limiter sharedlimiter.Limiter
	opts    *options
}

// Dependencies returns optional interceptors that should run before limiter.
func (i *interceptor) Dependencies() []string {
	return nil
}

// RequiredDependencies returns interceptors that limiter requires to function.
func (i *interceptor) RequiredDependencies() []string {
	return []string{"realip"}
}

// ServerInterceptor returns a new interceptor that limits the rate of incoming requests.
func ServerInterceptor(limiter sharedlimiter.Limiter, opt ...Option) interceptors.ServerInterceptor {
	opts := newOptions(opt...)

	return &interceptor{
		BaseInterceptor: interceptors.NewBaseInterceptorWithFilter(
			"limiter",
			opts.ignoreMethods,
			opts.ignorePatterns,
			opts.logger,
		),
		limiter: limiter,
		opts:    opts,
	}
}

// ServerUnaryInterceptor returns a new unary server interceptor that limits the rate of incoming requests.
func ServerUnaryInterceptor(limiter sharedlimiter.Limiter, opt ...Option) stdGrpc.UnaryServerInterceptor {
	return ServerInterceptor(limiter, opt...).ServerUnaryInterceptor()
}

// ServerStreamInterceptor returns a new streaming server interceptor that limits the rate of incoming requests.
func ServerStreamInterceptor(limiter sharedlimiter.Limiter, opt ...Option) stdGrpc.StreamServerInterceptor {
	return ServerInterceptor(limiter, opt...).ServerStreamInterceptor()
}

// ServerUnaryInterceptor returns a new unary server interceptor that limits the rate of incoming requests.
func (i *interceptor) ServerUnaryInterceptor() stdGrpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *stdGrpc.UnaryServerInfo, handler stdGrpc.UnaryHandler) (resp any, err error) {
		md, err := i.rateLimit(ctx, i.InternMethod(info.FullMethod))
		_ = stdGrpc.SetHeader(ctx, md) //nolint:errcheck
		if err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// ServerStreamInterceptor returns a new streaming server interceptor that limits the rate of incoming requests.
func (i *interceptor) ServerStreamInterceptor() stdGrpc.StreamServerInterceptor {
	return func(srv any, stream stdGrpc.ServerStream, info *stdGrpc.StreamServerInfo, handler stdGrpc.StreamHandler) error {
		md, err := i.rateLimit(stream.Context(), i.InternMethod(info.FullMethod))
		_ = stream.SetHeader(md) //nolint:errcheck
		if err != nil {
			return err
		}
		return handler(srv, stream)
	}
}

func (i *interceptor) rateLimit(ctx context.Context, method string) (metadata.MD, error) {
	md, _ := metadata.FromOutgoingContext(ctx)
	if md == nil {
		md = metadata.New(nil)
	}

	if i.ShouldIgnore(method) {
		i.LogIgnored(ctx, method)
		return md, nil
	}

	if ip := clientip.FromContext(ctx); ip.IsValid() {
		ctx = tokenbucket.ContextWithClientIP(ctx, ip.String())
	}

	if token := extractBearerToken(ctx); token != "" {
		ctx = tokenbucket.ContextWithAuthToken(ctx, token)
	}

	info, err := i.limiter.Limit(ctx)

	if info != nil {
		i.LogDebug(ctx, "setting rate limit headers", method,
			slog.Int64("limit", info.Limit),
			slog.Int64("remaining", info.Remaining),
			slog.Int64("reset", info.Reset))

		md.Set(RateLimitMetadataKey, strconv.FormatInt(info.Limit, 10))
		md.Set(RateLimitRemaining, strconv.FormatInt(info.Remaining, 10))
		md.Set(RateLimitReset, strconv.FormatInt(info.Reset, 10))
	}

	if err != nil {
		if errors.Is(err, sharedlimiter.ErrLimitExceeded) {
			i.LogDebug(ctx, "rate limit exceeded", method)
			return md, status.New(codes.ResourceExhausted, "Rate Limit Exceeded").Err()
		}

		i.LogWarn(ctx, "rate limit check failed", method, err)

		switch i.opts.fallbackBehavior {
		case fallback.Allow:
			return md, nil // Allow the request to proceed
		case fallback.Deny:
			return nil, interceptors.NewError(status.New(codes.Unavailable, "Service Temporary Unavailable"), nil)
		default:
			return nil, interceptors.NewError(status.New(codes.Internal, "Internal Error"), err)
		}
	}

	i.LogDebug(ctx, "rate limit check passed", method)
	return md, nil
}

func extractBearerToken(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}

	authHeaders := md.Get("authorization")
	if len(authHeaders) == 0 {
		return ""
	}

	authHeader := strings.TrimSpace(authHeaders[0])
	const bearerPrefix = "bearer "
	if len(authHeader) >= len(bearerPrefix) && strings.EqualFold(authHeader[:len(bearerPrefix)], bearerPrefix) {
		return strings.TrimSpace(authHeader[len(bearerPrefix):])
	}

	return ""
}
