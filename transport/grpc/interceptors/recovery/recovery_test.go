// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	stdGrpc "google.golang.org/grpc"
)

func TestServerInterceptor_Dependencies(t *testing.T) {
	i, ok := ServerInterceptor().(*interceptor) //nolint:errcheck
	require.True(t, ok, "unexpected type")
	deps := i.Dependencies()
	require.ElementsMatch(t, []string{"metadata", "requestid", "errstatus"}, deps)
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

// TestServerUnaryInterceptor_NoDefaultIgnore pins that recovery covers every
// method by default, including the health and reflection services: an
// unrecovered panic there would take the whole process down.
func TestServerUnaryInterceptor_NoDefaultIgnore(t *testing.T) {
	t.Parallel()

	for _, method := range []string{
		"/grpc.health.v1.Health/Check",
		"/grpc.health.v1.HealthAdmin/Reset",
		"/grpc.reflection.v1.ServerReflection/ServerReflectionInfo",
	} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()

			unary := ServerUnaryInterceptor(WithLogger(slog.New(slog.DiscardHandler)))
			var err error
			require.NotPanics(t, func() {
				_, err = unary(t.Context(), nil, &stdGrpc.UnaryServerInfo{FullMethod: method},
					func(context.Context, any) (any, error) { panic("boom") })
			})
			require.Error(t, err)
		})
	}
}
