// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errstatustest_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/errstatus"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/errstatus/errstatustest"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

const (
	domain = "svc"
	method = "/svc.test.v1.TestService/Do"
)

var harness = errstatustest.New(
	errstatus.WithCanonicalAPIRules(),
	errstatus.WithFinalizer(errstatus.DefaultFinalizerWithDomain(domain)),
)

// errorInfo returns the ErrorInfo detail of st, or nil.
func errorInfo(st *status.Status) *errdetails.ErrorInfo {
	for _, d := range st.Details() {
		if ei, ok := d.(*errdetails.ErrorInfo); ok {
			return ei
		}
	}
	return nil
}

func TestCall(t *testing.T) {
	t.Parallel()

	err := harness.Call(t, nil, method, func(context.Context) error {
		return fmt.Errorf("get thing: %w", coreerrs.ErrNotFound)
	})
	st := status.Convert(err)
	require.Equal(t, codes.NotFound, st.Code())
	require.Equal(t, "not found", st.Message())
	require.ErrorIs(t, err, coreerrs.ErrNotFound, "the cause must stay reachable")
	info := errorInfo(st)
	require.NotNil(t, info)
	require.Equal(t, "NOT_FOUND", info.GetReason())
	require.Equal(t, domain, info.GetDomain())

	require.NoError(t, harness.Call(t, nil, method, func(context.Context) error { return nil }))
}

func TestCallGetsTestContext(t *testing.T) {
	t.Parallel()

	err := harness.Call(t, nil, method, func(ctx context.Context) error { return ctx.Err() })
	require.NoError(t, err)
}

func TestUnary(t *testing.T) {
	t.Parallel()

	got, err := errstatustest.Unary(t, harness, nil, method, func(context.Context) (string, error) { return "ok", nil })
	require.NoError(t, err)
	require.Equal(t, "ok", got)

	got, err = errstatustest.Unary(t, harness, nil, method, func(context.Context) (string, error) {
		return "partial", errors.New("boom")
	})
	require.Empty(t, got)
	require.Equal(t, codes.Internal, status.Code(err))
}

// handler is a service with an errstatus.StatusError converter.
type handler struct{}

func (handler) StatusErrorConvert(_ context.Context, err error) error {
	if errors.Is(err, coreerrs.ErrBusy) {
		return status.Error(codes.FailedPrecondition, "busy")
	}
	return err
}

func (handler) Echo(_ context.Context, req string) (string, error) {
	if req == "" {
		return "", coreerrs.NewFieldError("message", "required")
	}
	return req, nil
}

func TestInvoke(t *testing.T) {
	t.Parallel()

	h := handler{}
	got, err := errstatustest.Invoke(t, harness, h, method, h.Echo, "hi")
	require.NoError(t, err)
	require.Equal(t, "hi", got)

	_, err = errstatustest.Invoke(t, harness, h, method, h.Echo, "")
	st := status.Convert(err)
	require.Equal(t, codes.InvalidArgument, st.Code())
	require.Equal(t, "message: required", st.Message())
}

func TestCallServiceConverter(t *testing.T) {
	t.Parallel()

	err := harness.Call(t, handler{}, method, func(context.Context) error { return coreerrs.ErrBusy })
	st := status.Convert(err)
	require.Equal(t, codes.FailedPrecondition, st.Code())
	require.Equal(t, "busy", st.Message())
}

func TestConvert(t *testing.T) {
	t.Parallel()

	require.NoError(t, harness.Convert(t, nil))
	// Convert has no service, so no service-level converter applies.
	st := status.Convert(harness.Convert(t, coreerrs.ErrBusy))
	require.Equal(t, codes.Aborted, st.Code())
	require.Equal(t, "BUSY", errorInfo(st).GetReason())
	// A field error is camel-cased by the API rules.
	st = status.Convert(harness.Convert(t, coreerrs.NewFieldError("base_url", "invalid")))
	require.Equal(t, "baseUrl: invalid", st.Message())
}
