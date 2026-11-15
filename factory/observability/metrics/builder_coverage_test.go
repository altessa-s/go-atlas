// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/metrics/adapters"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	observabilityconfig "github.com/altessa-s/go-atlas/config/observability"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

func TestBuild_UnknownTypeFails(t *testing.T) {
	t.Parallel()
	_, err := factory.New(&observabilityconfig.Metrics{Enabled: true, Type: "statsd"}).Build()
	require.ErrorIs(t, err, corefactory.ErrUnsupported)
}

func TestBuild_UnregisteredPrometheusNamesTheModule(t *testing.T) {
	t.Parallel()
	_, err := factory.New(&observabilityconfig.Metrics{Enabled: true, Type: observabilityconfig.MetricsTypePrometheus}).Build()
	require.ErrorContains(t, err, "factory/observability/metrics/adapters/prometheus")
}

func TestBuild_RegisteredAdapter(t *testing.T) {
	t.Parallel()
	var gotCfg *observabilityconfig.MetricsAdapters
	want := &observabilityconfig.MetricsAdapters{}
	c, err := factory.New(&observabilityconfig.Metrics{Enabled: true, Type: "custom", ServiceName: "svc", Adapters: want}).
		UseAdapter("custom", func(cfg *observabilityconfig.MetricsAdapters) (adapters.Adapter, error) {
			gotCfg = cfg
			return nil, nil
		}).Build()
	require.NoError(t, err)
	require.NotNil(t, c)
	require.Same(t, want, gotCfg)
}

func TestBuild_NilAdapterBuilderFails(t *testing.T) {
	t.Parallel()
	_, err := factory.New(&observabilityconfig.Metrics{Enabled: true, Type: "custom"}).UseAdapter("custom", nil).Build()
	require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)
}
