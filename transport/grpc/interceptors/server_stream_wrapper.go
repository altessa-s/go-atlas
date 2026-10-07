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

// NewServerWrappedStream creates stream with custom context. An existing
// wrapper is reused, its context updated in place, when d is nil or the
// wrapper has no driver yet (d is adopted); otherwise a new wrapper is nested
// so that every driver in the interceptor chain receives its stream hooks.
//
// Example:
//
//	stream = interceptors.NewServerWrappedStream(ctx, stream)
//	return handler(srv, stream)
func NewServerWrappedStream(ctx context.Context, stream grpc.ServerStream, d driver.Driver) *ServerStreamWrapper {
	if e, ok := stream.(*ServerStreamWrapper); ok && (d == nil || e.d == nil) {
		e.ctx = ctx
		if d != nil {
			e.d = d
		}
		return e
	}
	return &ServerStreamWrapper{ServerStream: stream, ctx: ctx, d: d}
}

// Context returns the wrapped context.
func (w *ServerStreamWrapper) Context() context.Context {
	return w.ctx
}

// SendMsg runs the driver's PreMsgSend hook, sends the message it returns and
// notifies the driver via PostMsgSent.
func (w *ServerStreamWrapper) SendMsg(m any) error {
	if ps, ok := w.d.(driver.DriverStreamPreSend); ok {
		var err error
		if m, err = ps.PreMsgSend(w.ctx, m); err != nil {
			return err
		}
	}
	if ds, ok := w.d.(driver.DriverStream); ok {
		return ds.PostMsgSent(w.ctx, m, w.ServerStream.SendMsg(m))
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
