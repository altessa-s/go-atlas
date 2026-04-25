// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package interceptors

import (
	"context"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/driver"

	"google.golang.org/grpc"
)

// ServerStreamWrapper intercepts server streaming operations.
type ServerStreamWrapper struct {
	grpc.ServerStream
	ctx context.Context
	d   driver.Driver
}

// NewServerWrappedStream creates stream with custom context.
// Updates in-place if already wrapped to avoid nesting.
//
// Example:
//
//	stream = interceptors.NewServerWrappedStream(ctx, stream)
//	return handler(srv, stream)
func NewServerWrappedStream(ctx context.Context, stream grpc.ServerStream, d driver.Driver) *ServerStreamWrapper {
	if e, ok := stream.(*ServerStreamWrapper); ok {
		e.ctx = ctx
		return e
	}
	return &ServerStreamWrapper{ServerStream: stream, ctx: ctx, d: d}
}

// Context returns the wrapped context.
func (w *ServerStreamWrapper) Context() context.Context {
	return w.ctx
}

// SendMsg sends message and notifies driver via PostMsgSent.
func (w *ServerStreamWrapper) SendMsg(m any) error {
	if w.d != nil {
		if ds, ok := w.d.(driver.DriverStream); ok {
			return ds.PostMsgSent(w.ctx, m, w.ServerStream.SendMsg(m))
		}
	}
	return w.ServerStream.SendMsg(m)
}

// RecvMsg receives message and calls driver's PostMsgReceive.
func (w *ServerStreamWrapper) RecvMsg(m any) error {
	if w.d != nil {
		if ds, ok := w.d.(driver.DriverStream); ok {
			err := w.ServerStream.RecvMsg(m)
			return ds.PostMsgReceive(w.ctx, m, err)
		}
	}
	return w.ServerStream.RecvMsg(m)
}
