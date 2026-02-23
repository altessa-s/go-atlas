// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package adapters

import (
	"errors"
	"sync/atomic"
	"testing"
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
			if got := tt.typ.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
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
	if m.Name() != "multi" {
		t.Errorf("Name() = %q", m.Name())
	}
}

func TestMultiAdapter_Register(t *testing.T) {
	a1 := &testAdapter{name: "a1"}
	a2 := &testAdapter{name: "a2"}
	m := NewMultiAdapter(a1, a2)

	if err := m.Register(&Desc{Name: "test"}); err != nil {
		t.Errorf("Register() = %v", err)
	}
}

func TestMultiAdapter_Register_Error(t *testing.T) {
	a1 := &testAdapter{name: "a1", registerErr: errors.New("fail")}
	m := NewMultiAdapter(a1)

	if err := m.Register(&Desc{Name: "test"}); err == nil {
		t.Error("expected error")
	}
}

func TestMultiAdapter_Records(t *testing.T) {
	a1 := &testAdapter{name: "a1"}
	a2 := &testAdapter{name: "a2"}
	m := NewMultiAdapter(a1, a2)

	m.RecordCounter("c", nil, 1)
	m.RecordGauge("g", nil, 2)
	m.RecordHistogram("h", nil, 3)

	if a1.counterCalls.Load() != 1 || a2.counterCalls.Load() != 1 {
		t.Error("counter not broadcast to all adapters")
	}
	if a1.gaugeCalls.Load() != 1 || a2.gaugeCalls.Load() != 1 {
		t.Error("gauge not broadcast to all adapters")
	}
	if a1.histCalls.Load() != 1 || a2.histCalls.Load() != 1 {
		t.Error("histogram not broadcast to all adapters")
	}
}

func TestMultiAdapter_Flush(t *testing.T) {
	a1 := &testAdapter{name: "a1"}
	m := NewMultiAdapter(a1)
	if err := m.Flush(); err != nil {
		t.Errorf("Flush() = %v", err)
	}
}

func TestMultiAdapter_Flush_Error(t *testing.T) {
	a1 := &testAdapter{name: "a1", flushErr: errors.New("fail")}
	m := NewMultiAdapter(a1)
	if err := m.Flush(); err == nil {
		t.Error("expected error")
	}
}

func TestMultiAdapter_Close(t *testing.T) {
	a1 := &testAdapter{name: "a1"}
	m := NewMultiAdapter(a1)
	if err := m.Close(); err != nil {
		t.Errorf("Close() = %v", err)
	}
}

func TestMultiAdapter_Close_Error(t *testing.T) {
	a1 := &testAdapter{name: "a1", closeErr: errors.New("fail")}
	m := NewMultiAdapter(a1)
	if err := m.Close(); err == nil {
		t.Error("expected error")
	}
}
