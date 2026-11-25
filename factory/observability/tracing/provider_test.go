// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/observability/tracing"
	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters"

	observabilityconfig "github.com/altessa-s/go-atlas/config/observability"
)

// The export worker serializes calls. The test reads events after each drain.
type lifecycleAdapter struct{ events []string }

func (*lifecycleAdapter) Name() string { return "lifecycle" }

func (a *lifecycleAdapter) ExportSpans(_ context.Context, spans []adapters.SpanData) error {
	for _, span := range spans {
		a.events = append(a.events, span.Name)
	}
	return nil
}

func (a *lifecycleAdapter) ForceFlush(context.Context) error {
	a.events = append(a.events, "flush")
	return nil
}

func (a *lifecycleAdapter) Shutdown(context.Context) error {
	a.events = append(a.events, "shutdown")
	return nil
}

func TestBuild_ProviderLifecycle(t *testing.T) {
	t.Parallel()
	exporter := &lifecycleAdapter{}
	provider, err := factory.New(stubTracing()).UseAdapter(stubType, func(context.Context, *observabilityconfig.TracingAdapters) (adapters.Adapter, error) {
		return exporter, nil
	}).Build(t.Context())
	require.NoError(t, err)
	defer func() { require.NoError(t, provider.Shutdown(t.Context())) }()

	scope := provider.WithScope("orders")
	_, ownsExporter := scope.(tracing.TracerProvider)
	require.False(t, ownsExporter)
	recorder := scope.Recorder("handler")
	_, span := recorder.Start(t.Context(), "before flush")
	span.End()
	require.NoError(t, provider.ForceFlush(t.Context()))
	require.Equal(t, []string{"before flush", "flush"}, exporter.events)

	_, span = recorder.Start(t.Context(), "before shutdown")
	span.End()
	require.NoError(t, provider.Shutdown(t.Context()))
	require.Equal(t, []string{"before flush", "flush", "before shutdown", "flush", "shutdown"}, exporter.events)
}

func TestBuild_DisabledProviderLifecycle(t *testing.T) {
	t.Parallel()
	provider, err := factory.New(nil).Build(t.Context())
	require.NoError(t, err)
	scope := provider.WithScope("orders")
	require.True(t, tracing.IsNoop(scope))
	_, ownsExporter := scope.(tracing.TracerProvider)
	require.False(t, ownsExporter)
	require.NoError(t, provider.ForceFlush(t.Context()))
	require.NoError(t, provider.Shutdown(t.Context()))
}
