// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package interceptors

import (
	"context"
	"testing"

	"google.golang.org/grpc"
)

func TestNoOpInterceptor_Name(t *testing.T) {
	i := &NoOpInterceptor{}
	if i.Name() != "noop" {
		t.Fatalf("Name() = %q", i.Name())
	}
}

func TestNoOpInterceptor_ServerUnaryInterceptor(t *testing.T) {
	i := &NoOpInterceptor{}
	interceptor := i.ServerUnaryInterceptor()
	resp, err := interceptor(t.Context(), "req", &grpc.UnaryServerInfo{}, func(ctx context.Context, req any) (any, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp != "ok" {
		t.Fatalf("resp = %v", resp)
	}
}

func TestNoOpInterceptor_ServerStreamInterceptor(t *testing.T) {
	i := &NoOpInterceptor{}
	interceptor := i.ServerStreamInterceptor()
	called := false
	err := interceptor(nil, nil, &grpc.StreamServerInfo{}, func(srv any, stream grpc.ServerStream) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("handler not called")
	}
}

func TestNoOpInterceptor_ImplementsServerInterceptor(t *testing.T) {
	var _ ServerInterceptor = (*NoOpInterceptor)(nil)
}

func TestNoOpClientInterceptor_Name(t *testing.T) {
	i := &NoOpClientInterceptor{}
	if i.Name() != "noop" {
		t.Fatalf("Name() = %q", i.Name())
	}
}

func TestNoOpClientInterceptor_ImplementsClientInterceptor(t *testing.T) {
	var _ ClientInterceptor = (*NoOpClientInterceptor)(nil)
}

func TestNoopDriver(t *testing.T) {
	d := NoopDriver()
	if d == nil {
		t.Fatal("NoopDriver returned nil")
	}
	resp, err := d.PreCall(t.Context(), nil)
	if resp != nil || err != nil {
		t.Fatalf("PreCall = (%v, %v)", resp, err)
	}
	if err := d.PostCall(t.Context(), nil, nil); err != nil {
		t.Fatal(err)
	}
}
