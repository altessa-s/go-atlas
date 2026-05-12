// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/metrics/adapters"
	"github.com/altessa-s/go-atlas/observability/metrics/adapters/memory"
)

func TestAdapter_Name(t *testing.T) {
	t.Parallel()
	require.Equal(t, "memory", memory.New().Name())
}

func TestAdapter_Counter(t *testing.T) {
	t.Parallel()
	a := memory.New()

	require.NoError(t, a.Register(&adapters.Desc{
		Name: "requests_total", Type: adapters.TypeCounter, LabelNames: []string{"method"},
	}))
	a.RecordCounter("requests_total", map[string]string{"method": "GET"}, 1)
	a.RecordCounter("requests_total", map[string]string{"method": "GET"}, 2)
	a.RecordCounter("requests_total", map[string]string{"method": "POST"}, 5)

	require.Equal(t, float64(3), a.CounterValue("requests_total", map[string]string{"method": "GET"}))
	require.Equal(t, float64(5), a.CounterValue("requests_total", map[string]string{"method": "POST"}))
	require.Equal(t, float64(0), a.CounterValue("requests_total", map[string]string{"method": "DELETE"}))
	require.Equal(t, float64(0), a.CounterValue("unknown", nil))
}

func TestAdapter_Gauge(t *testing.T) {
	t.Parallel()
	a := memory.New()

	require.NoError(t, a.Register(&adapters.Desc{Name: "in_flight", Type: adapters.TypeGauge}))
	a.RecordGauge("in_flight", nil, 1)
	a.RecordGauge("in_flight", nil, 5)
	a.RecordGauge("in_flight", nil, 3)

	require.Equal(t, float64(3), a.GaugeValue("in_flight", nil), "gauge keeps the latest value")
}

func TestAdapter_Histogram(t *testing.T) {
	t.Parallel()
	a := memory.New()

	require.NoError(t, a.Register(&adapters.Desc{
		Name: "duration_seconds", Type: adapters.TypeHistogram, LabelNames: []string{"path"},
	}))
	a.RecordHistogram("duration_seconds", map[string]string{"path": "/x"}, 0.1)
	a.RecordHistogram("duration_seconds", map[string]string{"path": "/x"}, 0.4)
	a.RecordHistogram("duration_seconds", map[string]string{"path": "/y"}, 1.0)

	require.Equal(t, uint64(2), a.HistogramCount("duration_seconds", map[string]string{"path": "/x"}))
	require.InDelta(t, 0.5, a.HistogramSum("duration_seconds", map[string]string{"path": "/x"}), 1e-9)
	require.Equal(t, uint64(1), a.HistogramCount("duration_seconds", map[string]string{"path": "/y"}))
}

func TestAdapter_LabelOrderDoesNotMatter(t *testing.T) {
	t.Parallel()
	a := memory.New()
	require.NoError(t, a.Register(&adapters.Desc{
		Name: "x_total", Type: adapters.TypeCounter, LabelNames: []string{"a", "b"},
	}))
	a.RecordCounter("x_total", map[string]string{"a": "1", "b": "2"}, 7)
	require.Equal(t, float64(7),
		a.CounterValue("x_total", map[string]string{"b": "2", "a": "1"}),
		"map iteration order must not affect lookup")
}

func TestAdapter_RegisterIdempotent(t *testing.T) {
	t.Parallel()
	a := memory.New()
	d := &adapters.Desc{Name: "x_total", Type: adapters.TypeCounter}
	require.NoError(t, a.Register(d))
	require.NoError(t, a.Register(d), "re-registration must not error")

	a.RecordCounter("x_total", nil, 1)
	a.RecordCounter("x_total", nil, 1)
	require.Equal(t, float64(2), a.CounterValue("x_total", nil),
		"re-registration must not reset state")
}

func TestAdapter_Exists(t *testing.T) {
	t.Parallel()
	a := memory.New()
	require.False(t, a.Exists("x"))
	require.NoError(t, a.Register(&adapters.Desc{Name: "x", Type: adapters.TypeCounter}))
	require.True(t, a.Exists("x"))
}

func TestAdapter_Names(t *testing.T) {
	t.Parallel()
	a := memory.New()
	require.NoError(t, a.Register(&adapters.Desc{Name: "b", Type: adapters.TypeCounter}))
	require.NoError(t, a.Register(&adapters.Desc{Name: "a", Type: adapters.TypeGauge}))
	require.Equal(t, []string{"a", "b"}, a.Names(), "Names returns sorted list")
}

func TestAdapter_Close(t *testing.T) {
	t.Parallel()
	a := memory.New()
	require.NoError(t, a.Register(&adapters.Desc{Name: "x", Type: adapters.TypeCounter}))
	a.RecordCounter("x", nil, 5)
	require.NoError(t, a.Close())
	require.Equal(t, float64(0), a.CounterValue("x", nil))
	require.False(t, a.Exists("x"))
}

func TestAdapter_RecordWithoutRegisterIsNoop(t *testing.T) {
	t.Parallel()
	a := memory.New()
	a.RecordCounter("ghost", nil, 1)
	a.RecordGauge("ghost", nil, 1)
	a.RecordHistogram("ghost", nil, 1)
	require.Equal(t, float64(0), a.CounterValue("ghost", nil))
}

func TestAdapter_Concurrent(t *testing.T) {
	t.Parallel()
	a := memory.New()
	require.NoError(t, a.Register(&adapters.Desc{Name: "x_total", Type: adapters.TypeCounter}))

	const goroutines = 16
	const perGoroutine = 1000

	var wg sync.WaitGroup
	for range goroutines {
		wg.Go(func() {
			for range perGoroutine {
				a.RecordCounter("x_total", nil, 1)
			}
		})
	}
	wg.Wait()

	require.Equal(t, float64(goroutines*perGoroutine), a.CounterValue("x_total", nil))
}
