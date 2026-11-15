// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errstatustest

import (
	"context"
	"testing"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/errstatus"

	"google.golang.org/grpc"
)

// ConvertMethod is the full method name [Harness.Convert] reports to the
// interceptor.
const ConvertMethod = "/errstatustest.v1.TestService/Convert"

// Harness runs handler calls through one errstatus server interceptor. It is
// safe for concurrent use, so one Harness may serve all parallel tests of a
// package.
type Harness struct {
	interceptor grpc.UnaryServerInterceptor
}

// New returns a Harness whose interceptor is built from opts, the errstatus
// options of the server under test (rules, field path mapper, finalizer).
func New(opts ...errstatus.Option) *Harness {
	return &Harness{interceptor: errstatus.ServerUnaryInterceptor(opts...)}
}

// Call runs fn as the unary RPC method (a full method name such as
// "/svc.bot.v1.BotService/GetBot") of the service srv and returns the error
// the client would see. srv is the handler, so its
// errstatus.StatusError converter, if any, applies. fn gets t.Context().
func (h *Harness) Call(t testing.TB, srv any, method string, fn func(ctx context.Context) error) error {
	t.Helper()
	_, err := Unary(t, h, srv, method, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, fn(ctx)
	})
	return err
}

// Convert returns the error a client would see for err, the result of a
// handler or helper called directly, like [Harness.Call] without a service
// (so no service-level converter). A nil err is nil.
func (h *Harness) Convert(t testing.TB, err error) error {
	t.Helper()
	return h.Call(t, nil, ConvertMethod, func(context.Context) error { return err })
}

// Unary is [Harness.Call] for a handler call that also returns a response;
// the response is the zero value when the call fails. It fails t when the
// interceptor returns a response of another type.
func Unary[T any](t testing.TB, h *Harness, srv any, method string, fn func(ctx context.Context) (T, error)) (T, error) {
	t.Helper()
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: method}
	resp, err := h.interceptor(t.Context(), nil, info, func(ctx context.Context, _ any) (any, error) {
		return fn(ctx)
	})
	if err != nil {
		var zero T
		return zero, err
	}
	out, ok := resp.(T)
	if !ok {
		t.Fatalf("errstatustest: response type %T, want %T", resp, out)
	}
	return out, nil
}

// Invoke runs the handler method fn (a method value such as h.GetBot) with
// req as the unary RPC method of srv, like [Unary]:
//
//	got, err := errstatustest.Invoke(t, harness, h, botv1.BotService_GetBot_FullMethodName, h.GetBot, req)
func Invoke[Req, Resp any](
	t testing.TB, h *Harness, srv any, method string, fn func(context.Context, Req) (Resp, error), req Req,
) (Resp, error) {
	t.Helper()
	return Unary(t, h, srv, method, func(ctx context.Context) (Resp, error) { return fn(ctx, req) })
}
