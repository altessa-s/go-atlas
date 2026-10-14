// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing_test

import (
	"context"
	"encoding/hex"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters"
	"github.com/altessa-s/go-atlas/observability/tracing/sampler"
)

type captureAdapter struct {
	*adapters.MultiAdapter
	mu    sync.Mutex
	spans []adapters.SpanData
}

func (a *captureAdapter) ExportSpans(_ context.Context, spans []adapters.SpanData) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.spans = append(a.spans, spans...)
	return nil
}

func hexID(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	require.NoError(t, err)
	return b
}

func TestRecorder_ExportCarriesIDsAndScope(t *testing.T) {
	t.Parallel()

	a := &captureAdapter{MultiAdapter: adapters.NewMultiAdapter()}
	tr := tracing.New(tracing.WithAdapter(a), tracing.WithSampler(sampler.AlwaysOn()))
	rec := tr.Recorder("svc", tracing.WithInstrumentationVersion("1.2.3"), tracing.WithSchemaURL("https://schema"))

	ctx, parent := rec.Start(t.Context(), "parent")
	_, child := rec.Start(ctx, "child", tracing.WithLinks(tracing.Link{SpanContext: parent.SpanContext()}))
	child.End()
	parent.End()

	require.Len(t, a.spans, 2)
	got := a.spans[0]
	psc := parent.SpanContext()

	require.Equal(t, hexID(t, psc.TraceID()), got.TraceID[:])
	require.Equal(t, hexID(t, psc.SpanID()), got.ParentID[:])
	require.Len(t, got.Links, 1)
	require.Equal(t, hexID(t, psc.TraceID()), got.Links[0].TraceID[:])
	require.Equal(t, hexID(t, psc.SpanID()), got.Links[0].SpanID[:])

	for _, s := range a.spans {
		require.NotNil(t, s.InstrumentationScope)
		require.Equal(t, adapters.InstrumentationScope{Name: "svc", Version: "1.2.3", SchemaURL: "https://schema"}, *s.InstrumentationScope)
	}
}
