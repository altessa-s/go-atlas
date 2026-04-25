// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"

	"google.golang.org/grpc"
)

func TestNew(t *testing.T) {
	srv, err := New()
	require.NoError(t, err)
	require.NotNil(t, srv, "server should not be nil")
}

func TestNew_WithReflection(t *testing.T) {
	srv, err := New(WithReflection())
	require.NoError(t, err)
	require.True(t, srv.reflection, "reflection should be enabled")
}

func TestServer_RegisterHandlers_Nil(t *testing.T) {
	srv, err := New()
	require.NoError(t, err)
	srv.RegisterHandlers(nil, nil)
	require.Len(t, srv.handlers, 0)
}

func TestServer_RegisterInterceptors_Empty(t *testing.T) {
	srv, err := New()
	require.NoError(t, err)
	err = srv.RegisterInterceptors()
	require.NoError(t, err)
}

func TestServer_IsStarted_Default(t *testing.T) {
	srv, err := New()
	require.NoError(t, err)
	require.False(t, srv.IsStarted(), "should not be started")
}

func TestServer_IsStopped_Default(t *testing.T) {
	srv, err := New()
	require.NoError(t, err)
	require.False(t, srv.IsStopped(), "should not be stopped initially")
}

func TestDefaultTimeoutConfig(t *testing.T) {
	cfg := DefaultTimeoutConfig()
	require.False(t, cfg.StartupVerification <= 0, "StartupVerification should be > 0")
}

// stubInterceptor is a minimal ServerInterceptor with a configurable name.
type stubInterceptor struct {
	name string
}

func (s *stubInterceptor) Name() string { return s.name }
func (s *stubInterceptor) ServerUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		return handler(ctx, req)
	}
}
func (s *stubInterceptor) ServerStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		return handler(srv, stream)
	}
}

var _ interceptors.ServerInterceptor = (*stubInterceptor)(nil)

func TestServer_RegisterInterceptors_MergesOnRepeatedCalls(t *testing.T) {
	srv, err := New()
	require.NoError(t, err)

	a := &stubInterceptor{name: "alpha"}
	b := &stubInterceptor{name: "bravo"}
	c := &stubInterceptor{name: "charlie"}

	// First call registers alpha.
	err = srv.RegisterInterceptors(a)
	require.NoError(t, err)
	got := len(srv.interceptors)
	require.Equal(t, 1, got)

	// Second call registers bravo and charlie — alpha must be preserved.
	err = srv.RegisterInterceptors(b, c)
	require.NoError(t, err)
	got = len(srv.interceptors)
	require.Equal(t, 3, got)

	// Verify all three are present by name.
	names := make(map[string]bool)
	for _, v := range srv.interceptors {
		si := v.(interceptors.ServerInterceptor)
		names[si.(interceptors.Interceptor).Name()] = true
	}
	for _, want := range []string{"alpha", "bravo", "charlie"} {
		require.True(t, names[want], "interceptor %q not found after merge", want)
	}
}

func TestServer_RegisterInterceptors_AccumulatesAcrossCalls(t *testing.T) {
	srv, err := New()
	require.NoError(t, err)

	a := &stubInterceptor{name: "alpha"}
	a2 := &stubInterceptor{name: "alpha"} // duplicate name

	err = srv.RegisterInterceptors(a)
	require.NoError(t, err)
	err = srv.RegisterInterceptors(a2)
	require.NoError(t, err)

	// Both are stored; deduplication happens later in ServerOptions().
	got := len(srv.interceptors)
	require.Equal(t, 2, got)
}
