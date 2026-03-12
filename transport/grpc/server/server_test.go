// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpc

import (
	"context"
	"testing"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"

	"google.golang.org/grpc"
)

func TestNew(t *testing.T) {
	srv, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if srv == nil {
		t.Fatal("server should not be nil")
	}
}

func TestNew_WithReflection(t *testing.T) {
	srv, err := New(WithReflection())
	if err != nil {
		t.Fatal(err)
	}
	if !srv.reflection {
		t.Fatal("reflection should be enabled")
	}
}

func TestServer_RegisterHandlers_Nil(t *testing.T) {
	srv, err := New()
	if err != nil {
		t.Fatal(err)
	}
	srv.RegisterHandlers(nil, nil)
	if len(srv.handlers) != 0 {
		t.Fatalf("handlers = %d, want 0 (nil filtered)", len(srv.handlers))
	}
}

func TestServer_RegisterInterceptors_Empty(t *testing.T) {
	srv, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.RegisterInterceptors(); err != nil {
		t.Fatal(err)
	}
}

func TestServer_IsStarted_Default(t *testing.T) {
	srv, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if srv.IsStarted() {
		t.Fatal("should not be started")
	}
}

func TestServer_IsStopped_Default(t *testing.T) {
	srv, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if srv.IsStopped() {
		t.Fatal("should not be stopped initially")
	}
}

func TestDefaultTimeoutConfig(t *testing.T) {
	cfg := DefaultTimeoutConfig()
	if cfg.StartupVerification <= 0 {
		t.Fatal("StartupVerification should be > 0")
	}
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
	if err != nil {
		t.Fatal(err)
	}

	a := &stubInterceptor{name: "alpha"}
	b := &stubInterceptor{name: "bravo"}
	c := &stubInterceptor{name: "charlie"}

	// First call registers alpha.
	if err := srv.RegisterInterceptors(a); err != nil {
		t.Fatal(err)
	}
	if got := len(srv.interceptors); got != 1 {
		t.Fatalf("after first call: interceptors = %d, want 1", got)
	}

	// Second call registers bravo and charlie — alpha must be preserved.
	if err := srv.RegisterInterceptors(b, c); err != nil {
		t.Fatal(err)
	}
	if got := len(srv.interceptors); got != 3 {
		t.Fatalf("after second call: interceptors = %d, want 3", got)
	}

	// Verify all three are present by name.
	names := make(map[string]bool)
	for _, v := range srv.interceptors {
		si := v.(interceptors.ServerInterceptor)
		names[si.(interceptors.Interceptor).Name()] = true
	}
	for _, want := range []string{"alpha", "bravo", "charlie"} {
		if !names[want] {
			t.Errorf("interceptor %q not found after merge", want)
		}
	}
}

func TestServer_RegisterInterceptors_AccumulatesAcrossCalls(t *testing.T) {
	srv, err := New()
	if err != nil {
		t.Fatal(err)
	}

	a := &stubInterceptor{name: "alpha"}
	a2 := &stubInterceptor{name: "alpha"} // duplicate name

	if err := srv.RegisterInterceptors(a); err != nil {
		t.Fatal(err)
	}
	if err := srv.RegisterInterceptors(a2); err != nil {
		t.Fatal(err)
	}

	// Both are stored; deduplication happens later in ServerOptions().
	if got := len(srv.interceptors); got != 2 {
		t.Fatalf("interceptors = %d, want 2 (dedup deferred to Start)", got)
	}
}
