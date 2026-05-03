// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

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

func TestServerInterceptor_WithCustomCollector(t *testing.T) {
	coll := testhelpers.NewTestCollector(prom.NewRegistry())
	i := ServerInterceptor(WithCollector(coll))
	require.NotNil(t, i, "should not be nil")
	require.Equal(t, "metrics", i.Name())
}

func TestServerInterceptor_Dependencies(t *testing.T) {
	coll := testhelpers.NewTestCollector(prom.NewRegistry())
	i, _ := ServerInterceptor(WithCollector(coll)).(*serverInterceptorWrapper) //nolint:errcheck
	deps := i.Dependencies()
	require.Len(t, deps, 1)
	require.Equal(t, "metadata", deps[0])
}

// TestServerInterceptor_TwoSubsystemsShareCollector locks the contract that
// two interceptors with distinct subsystems can register against the same
// Prometheus registry without panicking on duplicate metric registration —
// the bug that motivated the move from singleton + raw Registerer to
// metrics.Collector + WithMetricsSubsystem.
func TestServerInterceptor_TwoSubsystemsShareCollector(t *testing.T) {
	registry := prom.NewRegistry()
	coll := testhelpers.NewTestCollector(registry)

	require.NotPanics(t, func() {
		_ = ServerInterceptor(WithCollector(coll), WithMetricsSubsystem("egrul"))
		_ = ServerInterceptor(WithCollector(coll), WithMetricsSubsystem("kfocus"))
	})
}

// TestServerInterceptor_SameSubsystemReusesMetrics confirms the adapter
// dedupes by name: calling ServerInterceptor twice with the same collector
// and same subsystem reuses the existing metric vectors without panic.
func TestServerInterceptor_SameSubsystemReusesMetrics(t *testing.T) {
	registry := prom.NewRegistry()
	coll := testhelpers.NewTestCollector(registry)

	require.NotPanics(t, func() {
		_ = ServerInterceptor(WithCollector(coll))
		_ = ServerInterceptor(WithCollector(coll))
	})
}

func BenchmarkGetStatusCode(b *testing.B) {
	err := status.Error(codes.NotFound, "not found")
	for b.Loop() {
		getStatusCode(err)
	}
}
