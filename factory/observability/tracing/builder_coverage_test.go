// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/observability/tracing"
	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters/console"

	observabilityconfig "github.com/altessa-s/go-atlas/config/observability"
)

func enabledTracing(typ observabilityconfig.TracingType) *observabilityconfig.Tracing {
	cfg := observabilityconfig.DefaultTracing()
	cfg.Enabled = true
	cfg.Type = typ
	return &cfg
}

func TestBuild_NilOrDisabledReturnsNoop(t *testing.T) {
	t.Parallel()

	tr, err := factory.New(nil).Build(t.Context())
	require.NoError(t, err)
	require.True(t, tracing.IsNoop(tr))

	cfg := observabilityconfig.DefaultTracing()
	cfg.Enabled = false
	tr, err = factory.New(&cfg).Build(t.Context())
	require.NoError(t, err)
	require.True(t, tracing.IsNoop(tr))
}

func TestBuild_UnknownTypeFails(t *testing.T) {
	t.Parallel()

	_, err := factory.New(enabledTracing("zipkin")).Build(t.Context())
	require.ErrorContains(t, err, "zipkin")
}

func TestBuild_UnknownSamplerFails(t *testing.T) {
	t.Parallel()

	cfg := stubTracing()
	cfg.Sampler = &observabilityconfig.TracingSampler{Type: "sometimes"}
	_, err := factory.New(cfg).UseAdapter(stubType, stubBuilder).Build(t.Context())
	require.ErrorContains(t, err, "sometimes")
}

func TestBuild_NoopTypeReturnsNoop(t *testing.T) {
	t.Parallel()

	tr, err := factory.New(enabledTracing(observabilityconfig.TracingTypeNoop)).Build(t.Context())
	require.NoError(t, err)
	require.True(t, tracing.IsNoop(tr))
}

func TestBuild_SamplerSelection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sampler   *observabilityconfig.TracingSampler
		recording bool
	}{
		{"nil_defaults_to_always_on", nil, true},
		{"always_on", &observabilityconfig.TracingSampler{Type: observabilityconfig.SamplerTypeAlwaysOn}, true},
		{"always_off", &observabilityconfig.TracingSampler{Type: observabilityconfig.SamplerTypeAlwaysOff}, false},
		{"ratio_one", &observabilityconfig.TracingSampler{Type: observabilityconfig.SamplerTypeTraceIDRatio, Ratio: 1}, true},
		{"ratio_zero", &observabilityconfig.TracingSampler{Type: observabilityconfig.SamplerTypeTraceIDRatio, Ratio: 0}, false},
		{"parent_based_one", &observabilityconfig.TracingSampler{Type: observabilityconfig.SamplerTypeParentBased, Ratio: 1}, true},
		{"parent_based_zero", &observabilityconfig.TracingSampler{Type: observabilityconfig.SamplerTypeParentBased, Ratio: 0}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := stubTracing()
			cfg.Sampler = tc.sampler
			tr, err := factory.New(cfg).UseAdapter(stubType, stubBuilder).Build(t.Context())
			require.NoError(t, err)
			require.False(t, tracing.IsNoop(tr))
			t.Cleanup(func() { _ = tr.Shutdown(t.Context()) })

			_, span := tr.Recorder("test").Start(t.Context(), "op")
			defer span.End()
			require.Equal(t, tc.recording, span.IsRecording())
		})
	}
}

func TestBuild_ConsoleAdapter(t *testing.T) {
	t.Parallel()

	for _, adapters := range []*observabilityconfig.TracingAdapters{
		nil,
		{},
		{Console: &observabilityconfig.TracingConsole{PrettyPrint: true, Timestamps: true}},
	} {
		cfg := enabledTracing(observabilityconfig.TracingTypeConsole)
		cfg.Adapters = adapters
		// Sampling everything out keeps the console adapter from writing to stdout.
		cfg.Sampler = &observabilityconfig.TracingSampler{Type: observabilityconfig.SamplerTypeAlwaysOff}

		tr, err := factory.New(cfg).Build(t.Context())
		require.NoError(t, err)
		require.False(t, tracing.IsNoop(tr))
		require.NoError(t, tr.Shutdown(t.Context()))
	}
}

func TestUseLogger_ReturnsSameBuilder(t *testing.T) {
	t.Parallel()

	b := factory.New(nil)
	require.Same(t, b, b.UseLogger(slog.New(slog.DiscardHandler)))
	require.Same(t, b, b.UseDefaultLogger())
	require.Same(t, slog.Default(), b.Logger())
}

// stubType is a tracing type served by a registered builder that exports to
// a discarding console adapter.
const stubType observabilityconfig.TracingType = "stub"

var stubBuilder factory.AdapterBuilder = func(context.Context, *observabilityconfig.TracingAdapters) (adapters.Adapter, error) {
	return console.New(console.WithWriter(io.Discard)), nil
}

func stubTracing() *observabilityconfig.Tracing { return enabledTracing(stubType) }

func TestBuild_UnregisteredOTLPNamesTheModule(t *testing.T) {
	t.Parallel()
	_, err := factory.New(enabledTracing(observabilityconfig.TracingTypeOTLP)).Build(t.Context())
	require.ErrorContains(t, err, "factory/observability/tracing/adapters/otlp")
}

func TestBuild_NilAdapterBuilderFails(t *testing.T) {
	t.Parallel()
	_, err := factory.New(stubTracing()).UseAdapter(stubType, nil).Build(t.Context())
	require.ErrorContains(t, err, "nil builder")
}
