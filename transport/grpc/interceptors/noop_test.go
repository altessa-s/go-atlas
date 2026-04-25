// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package interceptors

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc"
)

func TestNoOpInterceptor_Name(t *testing.T) {
	i := &NoOpInterceptor{}
	require.Equal(t, "noop", i.Name())
}

func TestNoOpInterceptor_ServerUnaryInterceptor(t *testing.T) {
	i := &NoOpInterceptor{}
	interceptor := i.ServerUnaryInterceptor()
	resp, err := interceptor(t.Context(), "req", &grpc.UnaryServerInfo{}, func(ctx context.Context, req any) (any, error) {
		return "ok", nil
	})
	require.NoError(t, err)
	require.Equal(t, "ok", resp)
}

func TestNoOpInterceptor_ServerStreamInterceptor(t *testing.T) {
	i := &NoOpInterceptor{}
	interceptor := i.ServerStreamInterceptor()
	called := false
	err := interceptor(nil, nil, &grpc.StreamServerInfo{}, func(srv any, stream grpc.ServerStream) error {
		called = true
		return nil
	})
	require.NoError(t, err)
	require.True(t, called, "handler not called")
}

func TestNoOpInterceptor_ImplementsServerInterceptor(t *testing.T) {
	var _ ServerInterceptor = (*NoOpInterceptor)(nil)
}

func TestNoOpClientInterceptor_Name(t *testing.T) {
	i := &NoOpClientInterceptor{}
	require.Equal(t, "noop", i.Name())
}

func TestNoOpClientInterceptor_ImplementsClientInterceptor(t *testing.T) {
	var _ ClientInterceptor = (*NoOpClientInterceptor)(nil)
}

func TestNoopDriver(t *testing.T) {
	d := NoopDriver()
	require.NotNil(t, d, "NoopDriver returned nil")
	resp, err := d.PreCall(t.Context(), nil)
	require.Nil(t, resp)
	require.NoError(t, err)
	err = d.PostCall(t.Context(), nil, nil)
	require.NoError(t, err)
}
