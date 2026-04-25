// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package interceptors

import (
	"context"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/driver"

	"google.golang.org/grpc"
)

// ClientStreamWrapper wraps a [grpc.ClientStream] to intercept send, receive,
// and close operations through a [driver.Driver]. Each message send and
// receive is routed through the driver's PostMsgSent / PostMsgReceive hooks
// (when the driver also implements [driver.DriverStream]), enabling
// interceptors such as error-status conversion and logging to observe
// individual stream messages.
//
// The wrapper also overrides [grpc.ClientStream.Context] so that downstream
// interceptors see the enriched context rather than the original one.
type ClientStreamWrapper struct {
	grpc.ClientStream
	ctx context.Context
	d   driver.Driver
}

// NewClientStreamWrapper returns a [ClientStreamWrapper] that delegates
// streaming operations to d. If stream is already a *ClientStreamWrapper
// its context is updated in place and the existing wrapper is reused,
// avoiding unnecessary nesting.
func NewClientStreamWrapper(ctx context.Context, stream grpc.ClientStream, d driver.Driver) *ClientStreamWrapper {
	if e, ok := stream.(*ClientStreamWrapper); ok {
		e.ctx = ctx
		return e
	}
	return &ClientStreamWrapper{ClientStream: stream, ctx: ctx, d: d}
}

// Context returns the wrapped context.
func (w *ClientStreamWrapper) Context() context.Context {
	return w.ctx
}

// SendMsg sends message and notifies driver via PostMsgSent.
func (w *ClientStreamWrapper) SendMsg(m any) error {
	if ds, ok := w.d.(driver.DriverStream); ok {
		return ds.PostMsgSent(w.ctx, m, w.ClientStream.SendMsg(m))
	}
	return w.ClientStream.SendMsg(m)
}

// RecvMsg receives message and calls driver's PostMsgReceive.
func (w *ClientStreamWrapper) RecvMsg(m any) error {
	if ds, ok := w.d.(driver.DriverStream); ok {
		return ds.PostMsgReceive(w.ctx, m, w.ClientStream.RecvMsg(m))
	}
	return w.ClientStream.RecvMsg(m)
}

// CloseSend closes send direction and calls PostCall.
func (w *ClientStreamWrapper) CloseSend() error {
	err := w.ClientStream.CloseSend()
	return w.d.PostCall(w.ctx, nil, err)
}
