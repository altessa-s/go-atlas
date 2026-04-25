// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package prometheus

import (
	"testing"

	"github.com/stretchr/testify/require"

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
			got := getStatusCode(tt.err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestServerInterceptor_WithCustomRegistry(t *testing.T) {
	reg := prom.NewRegistry()
	i := ServerInterceptor(WithRegisterer(reg))
	require.NotNil(t, i, "should not be nil")
	require.Equal(t, "prometheus", i.Name())
}

func TestServerInterceptor_Dependencies(t *testing.T) {
	reg := prom.NewRegistry()
	i, _ := ServerInterceptor(WithRegisterer(reg)).(*serverInterceptorWrapper) //nolint:errcheck
	deps := i.Dependencies()
	require.Len(t, deps, 1)
	require.Equal(t, "metadata", deps[0])
}

func BenchmarkGetStatusCode(b *testing.B) {
	err := status.Error(codes.NotFound, "not found")
	for b.Loop() {
		getStatusCode(err)
	}
}
