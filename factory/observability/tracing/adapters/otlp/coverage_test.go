// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/observability/tracing/adapters/otlp"
	"github.com/altessa-s/go-atlas/observability/tracing"

	observabilityconfig "github.com/altessa-s/go-atlas/config/observability"
	proxyconfig "github.com/altessa-s/go-atlas/config/proxy"
	tracingfactory "github.com/altessa-s/go-atlas/factory/observability/tracing"
)

func enabledTracing(typ observabilityconfig.TracingType) *observabilityconfig.Tracing {
	cfg := observabilityconfig.DefaultTracing()
	cfg.Enabled = true
	cfg.Type = typ
	return &cfg
}

func TestBuild_OTLPDefaultsWhenAdapterConfigMissing(t *testing.T) {
	t.Parallel()

	cfg := enabledTracing(observabilityconfig.TracingTypeOTLP)
	cfg.Adapters = nil
	cfg.Sampler = &observabilityconfig.TracingSampler{Type: observabilityconfig.SamplerTypeAlwaysOff}

	tr, err := tracingfactory.New(cfg).UseAdapter(observabilityconfig.TracingTypeOTLP, factory.Builder()).Build(t.Context())
	require.NoError(t, err)
	require.False(t, tracing.IsNoop(tr))
	require.NoError(t, tr.Shutdown(t.Context()))
}

func TestBuild_OTLPInvalidProxyFails(t *testing.T) {
	t.Parallel()

	for _, protocol := range []observabilityconfig.OTLPProtocol{observabilityconfig.OTLPProtocolGRPC, observabilityconfig.OTLPProtocolHTTP} {
		cfg := enabledTracing(observabilityconfig.TracingTypeOTLP)
		cfg.Adapters = &observabilityconfig.TracingAdapters{OTLP: &observabilityconfig.TracingOTLP{
			Endpoint: "127.0.0.1:4317",
			Protocol: protocol,
			Insecure: true,
			Proxy:    &proxyconfig.Config{Mode: "bogus"},
		}}
		_, err := tracingfactory.New(cfg).UseAdapter(observabilityconfig.TracingTypeOTLP, factory.Builder()).Build(t.Context())
		require.Error(t, err, protocol)
	}
}
