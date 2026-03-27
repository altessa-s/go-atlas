// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errstatus

import (
	"context"
	"log/slog"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	stdGrpc "google.golang.org/grpc"
)

var _ interceptors.ClientInterceptor = (*clientInterceptor)(nil)

type clientInterceptor struct {
	interceptors.BaseInterceptor
	options *options
}

func (c *clientInterceptor) convertError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}

	c.LogDebug(ctx, "handling client error conversion", convertOperation,
		slog.Any("error", err),
		slog.String("error_type", errorTypeString(err)))

	st, ok := status.FromError(err)
	if !ok {
		c.LogDebug(ctx, "error is not a gRPC status, returning as-is", convertOperation,
			slog.Any("error", err))
		return err
	}

	c.LogDebug(ctx, "extracted gRPC status from error", convertOperation,
		slog.String("code", st.Code().String()),
		slog.String("message", st.Message()))

	// Try optimized index first (if available)
	if c.options.statusConverterIndex != nil {
		// Fast path: O(1) lookup by status code
		if converters, found := c.options.statusConverterIndex.fastPath[st.Code()]; found {
			for _, converter := range converters {
				if converter.Matcher(ctx, st) {
					c.LogDebug(ctx, "matched custom status converter (fast path)", convertOperation,
						slog.String("code", st.Code().String()))
					return converter.Convert(ctx, st)
				}
			}
		}

		// Slow path: complex matchers that inspect details
		for _, converter := range c.options.statusConverterIndex.slowPath {
			if converter.Matcher(ctx, st) {
				c.LogDebug(ctx, "matched custom status converter (slow path)", convertOperation,
					slog.String("code", st.Code().String()))
				return converter.Convert(ctx, st)
			}
		}
	} else {
		// Fallback: no index, use sequential search
		for _, converter := range c.options.statusConverter {
			if converter.Matcher(ctx, st) {
				c.LogDebug(ctx, "matched custom status converter", convertOperation,
					slog.String("code", st.Code().String()))
				return converter.Convert(ctx, st)
			}
		}
	}

	switch st.Code() {
	case codes.DeadlineExceeded:
		c.LogDebug(ctx, "converting DeadlineExceeded status to context error", convertOperation)
		return context.DeadlineExceeded
	case codes.Canceled:
		c.LogDebug(ctx, "converting Canceled status to context error", convertOperation)
		return context.Canceled
	default:
		c.LogDebug(ctx, "no status conversion matched, returning original gRPC error", convertOperation,
			slog.String("code", st.Code().String()))
	}

	return err
}

// ClientInterceptor returns a client interceptor that converts gRPC status errors
// into application errors using the configured status errorConverters.
func ClientInterceptor(opt ...Option) interceptors.ClientInterceptor {
	opts := newOptions(opt...)

	// Build optimized index for status errorConverters
	if len(opts.statusConverter) > 0 {
		opts.statusConverterIndex = buildStatusConverterIndex(opts.statusConverter)
	}

	return &clientInterceptor{
		BaseInterceptor: interceptors.NewBaseInterceptor(interceptorName, opts.logger),
		options:         opts,
	}
}

// ClientUnaryInterceptor returns a [stdGrpc.UnaryClientInterceptor] that
// converts gRPC status errors from unary responses using the configured
// [StatusConverter] chain. Conversion runs after the invoker returns.
func (c *clientInterceptor) ClientUnaryInterceptor() stdGrpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *stdGrpc.ClientConn, invoker stdGrpc.UnaryInvoker, opts ...stdGrpc.CallOption) error {
		err := invoker(ctx, method, req, reply, cc, opts...)
		return c.convertError(ctx, err)
	}
}

// ClientStreamInterceptor returns a [stdGrpc.StreamClientInterceptor] that
// converts gRPC status errors on streaming RPCs. Connection-level errors are
// converted immediately; per-message errors are converted through a
// [ClientStreamWrapper] that intercepts SendMsg and RecvMsg.
func (c *clientInterceptor) ClientStreamInterceptor() stdGrpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *stdGrpc.StreamDesc,
		cc *stdGrpc.ClientConn,
		method string,
		streamer stdGrpc.Streamer,
		opts ...stdGrpc.CallOption,
	) (stdGrpc.ClientStream, error) {
		stream, err := streamer(ctx, desc, cc, method, opts...)
		if err != nil {
			return nil, c.convertError(ctx, err)
		}

		return interceptors.NewClientStreamWrapper(ctx, stream, c), nil
	}
}

func (c *clientInterceptor) PreCall(_ context.Context, msg any) (any, error) {
	return msg, nil
}

func (c *clientInterceptor) PostCall(ctx context.Context, _ any, err error) error {
	return c.convertError(ctx, err)
}

func (c *clientInterceptor) PostMsgReceive(ctx context.Context, _ any, err error) error {
	return c.convertError(ctx, err)
}

func (c *clientInterceptor) PostMsgSent(ctx context.Context, _ any, err error) error {
	return c.convertError(ctx, err)
}
