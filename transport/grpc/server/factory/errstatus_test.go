// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/errstatus"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	grpcconfig "github.com/altessa-s/go-atlas/config/grpc"
	middlewareconfig "github.com/altessa-s/go-atlas/config/middleware"
	stdGrpc "google.golang.org/grpc"
)

func TestUseErrStatusOptions(t *testing.T) {
	t.Parallel()

	errQuota := errors.New("quota for tenant acme exhausted")
	cfg := &grpcconfig.Config{Interceptors: &grpcconfig.Interceptors{
		ErrStatus: &grpcconfig.ErrStatusInterceptor{
			BaseInterceptor: grpcconfig.BaseInterceptor{EnableMixin: middlewareconfig.EnableMixin{Enabled: true}},
			Domain:          new("svc.example.com"),
		},
	}}
	b := New(cfg).
		UseErrStatusOptions(errstatus.WithErrorRules(errstatus.ErrorRule{
			Target: errQuota, Code: codes.ResourceExhausted, Reason: "QUOTA_EXHAUSTED",
		})).
		WithErrStatusInterceptor()
	require.Empty(t, b.errs)
	require.Len(t, b.interceptors, 1)

	unary := b.interceptors[0].ServerUnaryInterceptor()
	_, err := unary(t.Context(), nil, &stdGrpc.UnaryServerInfo{FullMethod: "/svc.v1.S/M"},
		func(context.Context, any) (any, error) { return nil, errQuota })

	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.ResourceExhausted, st.Code())
	require.Equal(t, "ResourceExhausted", st.Message(), "the error text is not exposed")
	var reason, domain string
	for _, d := range st.Details() {
		if ei, ok := d.(*errdetails.ErrorInfo); ok {
			reason, domain = ei.GetReason(), ei.GetDomain()
		}
	}
	require.Equal(t, "QUOTA_EXHAUSTED", reason)
	require.Equal(t, "svc.example.com", domain, "the configured finalizer still applies")
}
