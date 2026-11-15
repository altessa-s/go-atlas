// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/transport/grpc/server"
	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"

	grpcconfig "github.com/altessa-s/go-atlas/config/grpc"
)

// The builder only creates instruments, so it accepts a lifecycle-free
// Meter such as a WithSubsystem scope and hands it back unchanged.
func TestServerBuilderMeter(t *testing.T) {
	t.Parallel()

	require.True(t, metrics.IsNoop(factory.New(&grpcconfig.Config{}).Meter()))

	var scoped metrics.Meter = metricstest.NewTestCollector().WithSubsystem("grpc_factory")
	b := factory.New(&grpcconfig.Config{}).UseMeter(scoped)
	require.Same(t, scoped, b.Meter())
}
