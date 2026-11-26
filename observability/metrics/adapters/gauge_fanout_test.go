// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package adapters_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/metrics/adapters"
	"github.com/altessa-s/go-atlas/observability/metrics/adapters/memory"
)

type gatedNativeGauge struct {
	*memory.Adapter
	set, release chan struct{}
}

func (a *gatedNativeGauge) RecordGauge(name string, labels map[string]string, value float64) {
	a.Adapter.RecordGauge(name, labels, value)
	close(a.set)
	<-a.release
}

func (a *gatedNativeGauge) AddGauge(name string, labels map[string]string, delta float64) {
	a.Adapter.RecordGauge(name, labels, a.GaugeValue(name, labels)+delta)
}

func TestMultiAdapter_GaugeFanoutUsesOneOrder(t *testing.T) {
	t.Parallel()

	for _, nativeOnly := range []bool{false, true} {
		name := "mixed"
		if nativeOnly {
			name = "native"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			first := &gatedNativeGauge{Adapter: memory.New(), set: make(chan struct{}), release: make(chan struct{})}
			second := memory.New()
			var child adapters.Adapter = second
			if nativeOnly {
				release := make(chan struct{})
				close(release)
				child = &gatedNativeGauge{Adapter: second, set: make(chan struct{}), release: release}
			}
			multi := adapters.NewMultiAdapter(first, child)
			require.NoError(t, multi.Register(&adapters.Desc{Name: "gauge", Type: adapters.TypeGauge}))

			setDone, addDone := make(chan struct{}), make(chan struct{})
			go func() {
				defer close(setDone)
				multi.RecordGauge("gauge", nil, 10)
			}()
			<-first.set
			go func() {
				defer close(addDone)
				multi.AddGauge("gauge", nil, 1)
			}()
			select {
			case <-addDone:
			case <-time.After(50 * time.Millisecond):
			}
			close(first.release)
			<-setDone
			<-addDone
			require.Equal(t, []float64{11, 11}, []float64{first.GaugeValue("gauge", nil), second.GaugeValue("gauge", nil)})
		})
	}
}
