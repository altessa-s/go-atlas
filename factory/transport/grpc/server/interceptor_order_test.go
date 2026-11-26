// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"

	grpcconfig "github.com/altessa-s/go-atlas/config/grpc"
)

func TestRecoveryDependencyOrder(t *testing.T) {
	t.Parallel()
	for _, logEnabled := range []bool{false, true} {
		for _, authEnabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("logger=%t/auth=%t", logEnabled, authEnabled), func(t *testing.T) {
				t.Parallel()
				cfg := &grpcconfig.Config{Interceptors: &grpcconfig.Interceptors{
					Logger:    &grpcconfig.LoggerInterceptor{Enabled: logEnabled},
					Auth:      &grpcconfig.AuthInterceptor{Enabled: authEnabled},
					ErrStatus: &grpcconfig.ErrStatusInterceptor{Enabled: true},
					Recovery:  &grpcconfig.RecoveryInterceptor{BaseInterceptor: enabled},
					FieldMask: &grpcconfig.FieldMaskInterceptor{BaseInterceptor: enabled},
				}}
				b := New(cfg).UseAuth(auth.AuthFunc(func(context.Context, auth.Request) (any, error) { return "user", nil }), nil).WithInterceptors()
				require.Empty(t, b.errs)

				// Register inner interceptors first to exercise the edges
				// independently of the factory's insertion order.
				chain := make([]any, 0, len(b.interceptors))
				for _, interceptor := range slices.Backward(b.interceptors) {
					chain = append(chain, interceptor)
				}
				order, err := interceptors.NewChain(chain...).DependencyOrder()
				require.NoError(t, err)
				errStatus := slices.Index(order, "errstatus")
				recovery := slices.Index(order, "recovery")
				require.GreaterOrEqual(t, errStatus, 0)
				require.Greater(t, recovery, errStatus)
				for _, inner := range []string{"fieldbehavior", "fieldmask", "protovalidator"} {
					require.Greater(t, slices.Index(order, inner), recovery, "%v", order)
				}
			})
		}
	}
}
