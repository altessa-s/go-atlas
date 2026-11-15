// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/transport/http/server"
	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"

	httpconfig "github.com/altessa-s/go-atlas/config/http"
)

// The builder only creates instruments, so it accepts a lifecycle-free
// Meter such as a WithSubsystem scope and hands it back unchanged.
func TestServerBuilderMeter(t *testing.T) {
	t.Parallel()

	require.True(t, metrics.IsNoop(factory.New(&httpconfig.Config{}).Meter()))

	var scoped metrics.Meter = metricstest.NewTestCollector().WithSubsystem("http_factory")
	b := factory.New(&httpconfig.Config{}).UseMeter(scoped)
	require.Same(t, scoped, b.Meter())
}
