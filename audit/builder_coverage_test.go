// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit_test

import (
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/audit"
	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"

	coreruntime "github.com/altessa-s/go-atlas-core/runtime"
)

// captureDispatcher records submitted events and accepts them unless full.
type captureDispatcher struct {
	events []*audit.Event
	full   bool
}

func (d *captureDispatcher) Submit(e *audit.Event) bool {
	if d.full {
		return false
	}
	d.events = append(d.events, e)
	return true
}

func (d *captureDispatcher) Dropped() int64 { return 0 }

func startedAuditor(t *testing.T, d audit.Dispatcher, opts ...audit.Option) *audit.Auditor {
	t.Helper()
	hooks := new(coreruntime.HookGroup)
	a, err := audit.New(d, append([]audit.Option{audit.WithShutdownHooks(hooks)}, opts...)...)
	require.NoError(t, err)
	require.NoError(t, a.Start())
	t.Cleanup(func() { _ = hooks.Shutdown(t.Context()) })
	return a
}

func TestEventBuilder_BuildSetsAllFields(t *testing.T) {
	t.Parallel()

	a, err := audit.New(&captureDispatcher{})
	require.NoError(t, err)

	ec := audit.EventContext{RequestID: "r", TraceID: "t", SpanID: "s", CorrelationID: "c"}
	md := map[string]any{"k": "v"}
	e := a.NewEvent(audit.EventTypeSystem, audit.ActionDelete).
		WithActor(audit.Actor{ID: "u"}).
		WithResource(audit.Resource{}).
		WithEventContext(ec).
		WithMetadata(md).
		WithFailure(403, "denied").
		Build()

	require.Equal(t, audit.EventTypeSystem, e.Type)
	require.Equal(t, audit.ActionDelete, e.Action)
	require.Equal(t, "u", e.Actor.ID)
	require.Equal(t, ec, e.Context)
	require.Equal(t, md, e.Metadata)
	require.Equal(t, audit.Result{Status: audit.ResultStatusFailure, Code: 403, Message: "denied"}, e.Result)
	require.Empty(t, e.ID, "Build does not fill defaults")
	require.True(t, e.Timestamp.IsZero())
}

func TestEventBuilder_ResultVariants(t *testing.T) {
	t.Parallel()

	a, err := audit.New(&captureDispatcher{})
	require.NoError(t, err)

	e := a.NewEvent(audit.EventTypeAuth, audit.ActionLogin).WithError(errors.New("db down")).Build()
	require.Equal(t, audit.ResultStatusError, e.Result.Status)
	require.NotNil(t, e.Result.Error)
	require.Equal(t, "db down", e.Result.Error.Message)

	custom := audit.Result{Status: audit.ResultStatusSuccess, Code: 201}
	e = a.NewEvent(audit.EventTypeAuth, audit.ActionLogin).WithError(errors.New("x")).WithResult(custom).Build()
	require.Equal(t, custom, e.Result, "the last result setter wins")
}

func TestEmit_FillsDefaultsAndServiceInfo(t *testing.T) {
	t.Parallel()

	d := &captureDispatcher{}
	svc := audit.ServiceInfo{Name: "svc", Version: "1"}
	a := startedAuditor(t, d, audit.WithServiceInfo(svc), audit.WithLogger(nil), audit.WithLogger(slog.New(slog.DiscardHandler)))

	before := time.Now()
	require.True(t, a.Emit(&audit.Event{Type: audit.EventTypeSystem}))
	require.Len(t, d.events, 1)
	e := d.events[0]
	require.NotEmpty(t, e.ID)
	require.False(t, e.Timestamp.Before(before))
	require.Equal(t, svc, e.Service)

	// Explicit values are kept.
	ts := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	own := audit.ServiceInfo{Name: "other"}
	require.True(t, a.Emit(&audit.Event{ID: "fixed", Timestamp: ts, Service: own}))
	require.Equal(t, "fixed", d.events[1].ID)
	require.Equal(t, ts, d.events[1].Timestamp)
	require.Equal(t, own, d.events[1].Service)

	require.False(t, a.Emit(nil))
}

func TestMetricsSubsystemOption_StringAndPointer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		opt    audit.Option
		metric string
	}{
		{"string", audit.WithMetricsSubsystem("sub1"), "test_sub1_events_emitted_total"},
		{"pointer", audit.WithMetricsSubsystem(new("sub2")), "test_sub2_events_emitted_total"},
		{"blank_keeps_default", audit.WithMetricsSubsystem("  "), "test_" + audit.DefaultMetricsSubsystem + "_events_emitted_total"},
		{"nil_pointer_keeps_default", audit.WithMetricsSubsystem((*string)(nil)), "test_" + audit.DefaultMetricsSubsystem + "_events_emitted_total"},
		{"blank_pointer_keeps_default", audit.WithMetricsSubsystem(new(" ")), "test_" + audit.DefaultMetricsSubsystem + "_events_emitted_total"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			col := metricstest.NewTestCollector()
			a := startedAuditor(t, &captureDispatcher{}, audit.WithMeter(col), tc.opt)
			require.True(t, a.Emit(&audit.Event{}))
			require.InDelta(t, 1.0, metricstest.GetCounterValue(t, col, tc.metric), 0)
		})
	}
}

func TestEmit_FullDispatcherDrops(t *testing.T) {
	t.Parallel()

	a := startedAuditor(t, &captureDispatcher{full: true})
	require.False(t, a.Emit(&audit.Event{}))
	require.False(t, a.NewEvent(audit.EventTypeSystem, audit.ActionRead).WithSuccess().Emit())
}

func TestShutdown_NotStartedIsNoop(t *testing.T) {
	t.Parallel()

	a, err := audit.New(&captureDispatcher{})
	require.NoError(t, err)
	require.NoError(t, a.Shutdown(t.Context()))
	require.False(t, a.Emit(&audit.Event{}))
}

func TestJSONCodec_RoundTrip(t *testing.T) {
	t.Parallel()

	var c audit.JSONCodec
	in := &audit.Event{
		ID:        "id",
		Type:      audit.EventTypeAuth,
		Action:    audit.ActionLogin,
		Timestamp: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Metadata:  map[string]any{"n": "x"},
	}
	b, err := c.Encode(in)
	require.NoError(t, err)

	out, err := c.Decode(b)
	require.NoError(t, err)
	require.Equal(t, in, out)

	_, err = c.Decode([]byte("{not json"))
	require.Error(t, err)
}

func TestJSONCodec_PreservesLargeIntegers(t *testing.T) {
	t.Parallel()

	const large = "9007199254740993"
	var c audit.JSONCodec
	in := &audit.Event{
		ID:        "evt-large",
		Type:      audit.EventTypeDataChange,
		Action:    audit.ActionUpdate,
		Timestamp: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Actor:     audit.Actor{Metadata: map[string]any{"actor_seq": json.Number(large)}},
		Resource: audit.Resource{
			Attributes: map[string]any{"res_seq": json.Number(large)},
			Changes: &audit.ResourceChanges{
				Before: map[string]any{"seq": json.Number(large)},
				After:  map[string]any{"seq": json.Number(large)},
			},
		},
		Metadata: map[string]any{"seq": json.Number(large)},
	}

	raw, err := c.Encode(in)
	require.NoError(t, err)

	decoded, err := c.Decode(raw)
	require.NoError(t, err)

	reencoded, err := c.Encode(decoded)
	require.NoError(t, err)
	require.NotContains(t, string(reencoded), "9007199254740992")
	require.Equal(t, string(raw), string(reencoded))

	_, err = c.Decode([]byte(`{"id":"a"}{"id":"b"}`))
	require.Error(t, err, "trailing JSON input must be rejected")
}
