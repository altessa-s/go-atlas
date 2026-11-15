// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
func TestServerInterceptor_WithCustomCollector(t *testing.T) {
	tc := metricstest.NewTestCollector()
	i := ServerInterceptor(WithMeter(tc))
	require.NotNil(t, i, "should not be nil")
	require.Equal(t, "metrics", i.Name())
}
func TestServerInterceptor_Dependencies(t *testing.T) {
	tc := metricstest.NewTestCollector()
	i, _ := ServerInterceptor(WithMeter(tc)).(*serverInterceptorWrapper) //nolint:errcheck
	deps := i.Dependencies()
	require.Len(t, deps, 1)
	require.Equal(t, "metadata", deps[0])
}

// TestServerInterceptor_TwoSubsystemsShareCollector locks the contract that
// two interceptors with distinct subsystems can register against the same
// Prometheus registry without panicking on duplicate metric registration —
// the bug that motivated the move from singleton + raw Registerer to
// metrics.Meter + WithMetricsSubsystem.
func TestServerInterceptor_TwoSubsystemsShareCollector(t *testing.T) {
	tc := metricstest.NewTestCollector()

	require.NotPanics(t, func() {
		_ = ServerInterceptor(WithMeter(tc), WithMetricsSubsystem("egrul"))
		_ = ServerInterceptor(WithMeter(tc), WithMetricsSubsystem("kfocus"))
	})
}

// TestServerInterceptor_SameSubsystemReusesMetrics confirms the adapter
// dedupes by name: calling ServerInterceptor twice with the same collector
// and same subsystem reuses the existing metric vectors without panic.
func TestServerInterceptor_SameSubsystemReusesMetrics(t *testing.T) {
	tc := metricstest.NewTestCollector()

	require.NotPanics(t, func() {
		_ = ServerInterceptor(WithMeter(tc))
		_ = ServerInterceptor(WithMeter(tc))
	})
}

// TestServerInterceptor_WithoutCollector pins the safe no-op default:
// omitting WithMeter must not panic.
func TestServerInterceptor_WithoutCollector(t *testing.T) {
	t.Parallel()

	require.NotPanics(t, func() { ServerInterceptor() })
}
