// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestServerInterceptor_Dependencies(t *testing.T) {
	i, ok := ServerInterceptor().(*interceptor) //nolint:errcheck
	require.True(t, ok, "unexpected type")
	deps := i.Dependencies()
	require.Len(t, deps, 2)
	require.Equal(t, "metadata", deps[0])
	require.Equal(t, "requestid", deps[1])
}

func TestServerInterceptor_Name(t *testing.T) {
	i := ServerInterceptor()
	require.Equal(t, "recovery", i.Name())
}

func TestClientInterceptor_Name(t *testing.T) {
	i := ClientInterceptor()
	require.Equal(t, "recovery", i.Name())
}

func TestClientInterceptor_Dependencies(t *testing.T) {
	i, ok := ClientInterceptor().(*interceptor) //nolint:errcheck
	require.True(t, ok, "unexpected type")
	deps := i.Dependencies()
	require.Len(t, deps, 2)
}

func TestServerInterceptor_ReturnsInterceptors(t *testing.T) {
	i := ServerInterceptor()
	require.NotNil(t, i.ServerUnaryInterceptor(), "ServerUnaryInterceptor should not be nil")
	require.NotNil(t, i.ServerStreamInterceptor(), "ServerStreamInterceptor should not be nil")
}

func TestClientInterceptor_ReturnsInterceptors(t *testing.T) {
	i := ClientInterceptor()
	require.NotNil(t, i.ClientUnaryInterceptor(), "ClientUnaryInterceptor should not be nil")
	require.NotNil(t, i.ClientStreamInterceptor(), "ClientStreamInterceptor should not be nil")
}
