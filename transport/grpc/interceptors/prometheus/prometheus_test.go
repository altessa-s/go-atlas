// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package prometheus

import (
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	prom "github.com/prometheus/client_golang/prometheus"
)

func TestGetStatusCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want codes.Code
	}{
		{"nil", nil, codes.OK},
		{"grpc_not_found", status.Error(codes.NotFound, "not found"), codes.NotFound},
		{"grpc_internal", status.Error(codes.Internal, "internal"), codes.Internal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getStatusCode(tt.err); got != tt.want {
				t.Fatalf("getStatusCode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestServerInterceptor_WithCustomRegistry(t *testing.T) {
	reg := prom.NewRegistry()
	i := ServerInterceptor(WithRegisterer(reg))
	if i == nil {
		t.Fatal("should not be nil")
	}
	if i.Name() != "prometheus" {
		t.Fatalf("Name = %q", i.Name())
	}
}

func TestServerInterceptor_Dependencies(t *testing.T) {
	reg := prom.NewRegistry()
	i, _ := ServerInterceptor(WithRegisterer(reg)).(*serverInterceptorWrapper) //nolint:errcheck
	deps := i.Dependencies()
	if len(deps) != 1 || deps[0] != "metadata" {
		t.Fatalf("Dependencies = %v", deps)
	}
}

func BenchmarkGetStatusCode(b *testing.B) {
	err := status.Error(codes.NotFound, "not found")
	for b.Loop() {
		getStatusCode(err)
	}
}
