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

func TestMultiAdapter_IteratesAllChildrenOnError(t *testing.T) {
	t.Parallel()

	regErr1 := errors.New("reg-1")
	regErr2 := errors.New("reg-2")
	flushErr1 := errors.New("flush-1")
	flushErr2 := errors.New("flush-2")
	closeErr1 := errors.New("close-1")
	closeErr2 := errors.New("close-2")

	a1 := &testAdapter{name: "a1", registerErr: regErr1, flushErr: flushErr1, closeErr: closeErr1}
	a2 := &testAdapter{name: "a2", registerErr: regErr2, flushErr: flushErr2, closeErr: closeErr2}
	m := NewMultiAdapter(a1, a2)

	err := m.Register(&Desc{Name: "m", Type: TypeCounter})
	require.ErrorIs(t, err, regErr1)
	require.ErrorIs(t, err, regErr2)

	err = m.Flush()
	require.ErrorIs(t, err, flushErr1)
	require.ErrorIs(t, err, flushErr2)

	err = m.Close()
	require.ErrorIs(t, err, closeErr1)
	require.ErrorIs(t, err, closeErr2)
}

func TestMultiAdapter_DescriptorConflict(t *testing.T) {
	t.Parallel()

	m := NewMultiAdapter(&testAdapter{name: "a1"})
	require.NoError(t, m.Register(&Desc{Name: "x", Type: TypeCounter, LabelNames: []string{"a"}}))
	require.NoError(t, m.Register(&Desc{Name: "x", Type: TypeCounter, LabelNames: []string{"a"}}))

	err := m.Register(&Desc{Name: "x", Type: TypeGauge, LabelNames: []string{"a"}})
	require.ErrorIs(t, err, ErrDescriptorConflict)

	err = m.Register(&Desc{Name: "x", Type: TypeCounter, LabelNames: []string{"b"}})
	require.ErrorIs(t, err, ErrDescriptorConflict)
}

func TestMultiAdapter_ZeroValueRegisters(t *testing.T) {
	t.Parallel()

	var multi MultiAdapter
	require.NoError(t, multi.Register(&Desc{Name: "gauge", Type: TypeGauge}))
	require.ErrorIs(t, multi.Register(&Desc{Name: "gauge", Type: TypeCounter}), ErrDescriptorConflict)
}
