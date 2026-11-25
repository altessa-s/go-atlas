// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpcstatus

import (
	"context"
	"log/slog"

	"github.com/altessa-s/go-atlas/internal/diagnostics"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var _ interceptors.ClientInterceptor = (*interceptor)(nil)

const (
	// Name identifies the status converter in an interceptor chain.
	Name             = "errstatus"
	convertOperation = "convert"
)

// ConvertFunc converts a recognized gRPC status. When matched is false,
// the interceptor applies its standard conversion for context errors.
type ConvertFunc func(ctx context.Context, st *status.Status) (err error, matched bool)

type interceptor struct {
	interceptors.BaseInterceptor
	convert ConvertFunc
}

// New returns a unary and streaming client interceptor. Pass nil convert to
// use the standard conversion for context errors.
func New(logger *slog.Logger, convert ConvertFunc) interceptors.ClientInterceptor {
	return &interceptor{
		BaseInterceptor: interceptors.NewBaseInterceptor(Name, logger),
		convert:         convert,
	}
}

func (c *interceptor) convertError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}

	class := slog.String("classification", diagnostics.ErrorClass(err))
	c.LogDebug(ctx, "handling client error conversion", convertOperation, class)

	st, ok := status.FromError(err)
	if !ok {
		c.LogDebug(ctx, "error is not a gRPC status, returning as-is", convertOperation, class)
		return err
	}

	c.LogDebug(ctx, "extracted gRPC status from error", convertOperation,
		slog.String("code", st.Code().String()))

	if c.convert != nil {
		if converted, matched := c.convert(ctx, st); matched {
			c.LogDebug(ctx, "matched custom status converter", convertOperation,
				slog.String("code", st.Code().String()))
			return converted
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

// ClientUnaryInterceptor returns a [grpc.UnaryClientInterceptor] that
// converts gRPC status errors from unary responses using the configured
// conversion function. Conversion runs after the invoker returns.
func (c *interceptor) ClientUnaryInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		err := invoker(ctx, method, req, reply, cc, opts...)
		return c.convertError(ctx, err)
	}
}

// ClientStreamInterceptor returns a [grpc.StreamClientInterceptor] that
// converts gRPC status errors on streaming RPCs. Connection-level errors are
// converted immediately; per-message errors are converted through the
// [interceptors.ClientStreamWithDriver] wrapper that intercepts SendMsg and RecvMsg.
func (c *interceptor) ClientStreamInterceptor() grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		stream, err := streamer(ctx, desc, cc, method, opts...)
		if err != nil {
			return nil, c.convertError(ctx, err)
		}

		return interceptors.ClientStreamWithDriver(ctx, stream, c), nil
	}
}

func (c *interceptor) PreCall(_ context.Context, msg any) (any, error) {
	return msg, nil
}

func (c *interceptor) PostCall(ctx context.Context, _ any, err error) error {
	return c.convertError(ctx, err)
}

func (c *interceptor) PostMsgReceive(ctx context.Context, _ any, err error) error {
	return c.convertError(ctx, err)
}

func (c *interceptor) PostMsgSent(ctx context.Context, _ any, err error) error {
	return c.convertError(ctx, err)
}
