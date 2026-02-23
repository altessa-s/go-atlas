// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

import (
	"testing"
)

func TestServerInterceptor_Dependencies(t *testing.T) {
	i, ok := ServerInterceptor().(*interceptor) //nolint:errcheck
	if !ok {
		t.Fatal("unexpected type")
	}
	deps := i.Dependencies()
	if len(deps) != 2 {
		t.Fatalf("Dependencies len = %d, want 2", len(deps))
	}
	if deps[0] != "metadata" {
		t.Fatalf("deps[0] = %q", deps[0])
	}
	if deps[1] != "requestid" {
		t.Fatalf("deps[1] = %q", deps[1])
	}
}

func TestServerInterceptor_Name(t *testing.T) {
	i := ServerInterceptor()
	if i.Name() != "recovery" {
		t.Fatalf("Name = %q", i.Name())
	}
}

func TestClientInterceptor_Name(t *testing.T) {
	i := ClientInterceptor()
	if i.Name() != "recovery" {
		t.Fatalf("Name = %q", i.Name())
	}
}

func TestClientInterceptor_Dependencies(t *testing.T) {
	i, ok := ClientInterceptor().(*interceptor) //nolint:errcheck
	if !ok {
		t.Fatal("unexpected type")
	}
	deps := i.Dependencies()
	if len(deps) != 2 {
		t.Fatalf("Dependencies len = %d, want 2", len(deps))
	}
}

func TestServerInterceptor_ReturnsInterceptors(t *testing.T) {
	i := ServerInterceptor()
	if i.ServerUnaryInterceptor() == nil {
		t.Fatal("ServerUnaryInterceptor should not be nil")
	}
	if i.ServerStreamInterceptor() == nil {
		t.Fatal("ServerStreamInterceptor should not be nil")
	}
}

func TestClientInterceptor_ReturnsInterceptors(t *testing.T) {
	i := ClientInterceptor()
	if i.ClientUnaryInterceptor() == nil {
		t.Fatal("ClientUnaryInterceptor should not be nil")
	}
	if i.ClientStreamInterceptor() == nil {
		t.Fatal("ClientStreamInterceptor should not be nil")
	}
}
