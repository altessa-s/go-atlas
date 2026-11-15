// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/metrics"

	"google.golang.org/grpc"
)

// TestServerUnaryInterceptor_MethodLabelIsBounded pins that method names past
// the configured limit, which a client controls behind an unknown-service
// handler, collapse into one overflow label instead of minting new series.
func TestServerUnaryInterceptor_MethodLabelIsBounded(t *testing.T) {
	t.Parallel()

	const (
		limit = 4
		total = 64
	)
	tc := metricstest.NewTestCollector()
	inter := metrics.ServerInterceptor(metrics.WithMeter(tc), metrics.WithMaxMethods(limit)).ServerUnaryInterceptor()
	handler := func(context.Context, any) (any, error) { return "ok", nil }
	method := func(n int) string { return fmt.Sprintf("/attacker.Service/Method%d", n) }

	var wg sync.WaitGroup
	for n := range total {
		wg.Go(func() {
			_, err := inter(t.Context(), "req", &grpc.UnaryServerInfo{FullMethod: method(n)}, handler)
			require.NoError(t, err)
		})
	}
	wg.Wait()

	tracked := 0
	for n := range total {
		if metricstest.GetCounterValue(t, tc, "test_grpc_server_requests_total", "method", method(n), "status", "OK") > 0 {
			tracked++
		}
	}
	require.Equal(t, limit, tracked, "distinct method labels must stop at the limit")
	require.Equal(t, float64(total-limit), metricstest.GetCounterValue(t, tc, "test_grpc_server_requests_total",
		"method", metrics.OverflowMethod, "status", "OK"), "methods past the limit must be recorded under the overflow label")
}
