// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package adapters

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMetricType_String(t *testing.T) {
	tests := []struct {
		typ  MetricType
		want string
	}{
		{TypeCounter, "counter"},
		{TypeGauge, "gauge"},
		{TypeHistogram, "histogram"},
		{MetricType(99), "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			require.Equal(t, tt.want, tt.typ.String())
		})
	}
}

type testAdapter struct {
	name         string
	registerErr  error
	flushErr     error
	closeErr     error
	counterCalls atomic.Int64
	gaugeCalls   atomic.Int64
	histCalls    atomic.Int64
}

func (a *testAdapter) Name() string                                             { return a.name }
func (a *testAdapter) Register(_ *Desc) error                                   { return a.registerErr }
func (a *testAdapter) RecordCounter(_ string, _ map[string]string, _ float64)   { a.counterCalls.Add(1) }
func (a *testAdapter) RecordGauge(_ string, _ map[string]string, _ float64)     { a.gaugeCalls.Add(1) }
func (a *testAdapter) RecordHistogram(_ string, _ map[string]string, _ float64) { a.histCalls.Add(1) }
func (a *testAdapter) Flush() error                                             { return a.flushErr }
func (a *testAdapter) Close() error                                             { return a.closeErr }

func TestMultiAdapter_Name(t *testing.T) {
	m := NewMultiAdapter()
	require.Equal(t, "multi", m.Name())
}

func TestMultiAdapter_Register(t *testing.T) {
	a1 := &testAdapter{name: "a1"}
	a2 := &testAdapter{name: "a2"}
	m := NewMultiAdapter(a1, a2)

	require.NoError(t, m.Register(&Desc{Name: "test"}))
}

func TestMultiAdapter_Register_Error(t *testing.T) {
	a1 := &testAdapter{name: "a1", registerErr: errors.New("fail")}
	m := NewMultiAdapter(a1)

	require.Error(t, m.Register(&Desc{Name: "test"}))
}

func TestMultiAdapter_Records(t *testing.T) {
	a1 := &testAdapter{name: "a1"}
	a2 := &testAdapter{name: "a2"}
	m := NewMultiAdapter(a1, a2)

	m.RecordCounter("c", nil, 1)
	m.RecordGauge("g", nil, 2)
	m.RecordHistogram("h", nil, 3)

	require.Equal(t, int64(1), a1.counterCalls.Load(), "counter not broadcast to a1")
	require.Equal(t, int64(1), a2.counterCalls.Load(), "counter not broadcast to a2")
	require.Equal(t, int64(1), a1.gaugeCalls.Load(), "gauge not broadcast to a1")
	require.Equal(t, int64(1), a2.gaugeCalls.Load(), "gauge not broadcast to a2")
	require.Equal(t, int64(1), a1.histCalls.Load(), "histogram not broadcast to a1")
	require.Equal(t, int64(1), a2.histCalls.Load(), "histogram not broadcast to a2")
}

func TestMultiAdapter_Flush(t *testing.T) {
	a1 := &testAdapter{name: "a1"}
	m := NewMultiAdapter(a1)
	require.NoError(t, m.Flush())
}

func TestMultiAdapter_Flush_Error(t *testing.T) {
	a1 := &testAdapter{name: "a1", flushErr: errors.New("fail")}
	m := NewMultiAdapter(a1)
	require.Error(t, m.Flush())
}

func TestMultiAdapter_Close(t *testing.T) {
	a1 := &testAdapter{name: "a1"}
	m := NewMultiAdapter(a1)
	require.NoError(t, m.Close())
}

func TestMultiAdapter_Close_Error(t *testing.T) {
	a1 := &testAdapter{name: "a1", closeErr: errors.New("fail")}
	m := NewMultiAdapter(a1)
	require.Error(t, m.Close())
}
