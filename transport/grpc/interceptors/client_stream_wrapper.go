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
// streaming operations to d. An existing *ClientStreamWrapper is reused, its
// context updated in place, when d is nil or the wrapper has no driver yet
// (d is adopted); otherwise a new wrapper is nested so that every driver
// receives its stream hooks.
func NewClientStreamWrapper(ctx context.Context, stream grpc.ClientStream, d driver.Driver) *ClientStreamWrapper {
	if e, ok := stream.(*ClientStreamWrapper); ok && (d == nil || e.d == nil) {
		e.ctx = ctx
		if d != nil {
			e.d = d
		}
		return e
	}
	return &ClientStreamWrapper{ClientStream: stream, ctx: ctx, d: d}
}

// Context returns the wrapped context.
func (w *ClientStreamWrapper) Context() context.Context {
	return w.ctx
}

// SendMsg runs the driver's PreMsgSend hook, sends the message it returns and
// notifies the driver via PostMsgSent.
func (w *ClientStreamWrapper) SendMsg(m any) error {
	if ps, ok := w.d.(driver.DriverStreamPreSend); ok {
		var err error
		if m, err = ps.PreMsgSend(w.ctx, m); err != nil {
			return err
		}
	}
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
	if w.d == nil {
		return err
	}
	return w.d.PostCall(w.ctx, nil, err)
}
