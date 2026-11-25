// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package opa_test

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/opa"
	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"
)

type pausedInput struct{ entered, release chan struct{} }

func (p pausedInput) MarshalJSON() ([]byte, error) {
	p.entered <- struct{}{}
	<-p.release
	return []byte(`{"user":"alice"}`), nil
}
func TestSnapshotDuringEvaluation(t *testing.T) {
	t.Parallel()
	src := &mutableSource{bundle: adminsBundle("alice")}
	revision := src.bundle.Revision
	sink := &captureSink{}
	m, err := opa.NewManager(t.Context(), src, "data.t.allow", opa.WithAuditRecorder(sink), opa.WithDecisionCache(2, time.Hour))
	require.NoError(t, err)
	defer m.Close()
	input := pausedInput{make(chan struct{}, 2), make(chan struct{})}
	done := make(chan error, 1)
	go func() {
		_, err := m.Evaluator().Evaluate(t.Context(), input)
		done <- err
	}()
	<-input.entered
	src.set(adminsBundle("bob"), nil)
	require.NoError(t, m.RunUpdateCycle(t.Context()))
	close(input.release)
	require.NoError(t, <-done)
	ds := sink.all()
	require.Len(t, ds, 1)
	require.True(t, ds[0].Allowed)
	require.Equal(t, revision, ds[0].Attributes["revision"])
	require.Empty(t, input.entered)
}

func TestSnapshotByteExactInput(t *testing.T) {
	t.Parallel()
	for _, opts := range [][]opa.Option{nil, {opa.WithDecisionCache(2, time.Hour)}} {
		src := &mutableSource{bundle: opa.NewPolicyBundle(map[string][]byte{"p.rego": []byte("package t")})}
		m, err := opa.NewManager(t.Context(), src, `base64.encode(input.v[0]) == "/w=="`, opts...)
		require.NoError(t, err)
		for _, b := range []byte{255, 254, 255} {
			r, err := m.Evaluator().Evaluate(t.Context(), map[string]any{"v": []any{string([]byte{b})}})
			require.NoError(t, err)
			require.Equal(t, b == 255, r.Allow)
		}
		require.NoError(t, m.Close())
	}
}

func TestSnapshotTimeDependency(t *testing.T) {
	t.Parallel()
	const policy = `package t
now := time.now_ns()
allow if now < input
constant := true
`
	for _, query := range []string{"time.now_ns() < input", "data.t.allow", "data.t.constant"} {
		t.Run(query, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				src := &mutableSource{bundle: opa.NewPolicyBundle(map[string][]byte{"p.rego": []byte(policy)})}
				collector := metricstest.NewTestCollector()
				m, err := opa.NewManager(t.Context(), src, query, opa.WithMeter(collector), opa.WithDecisionCache(2, time.Hour))
				require.NoError(t, err)
				deadline := time.Now().Add(time.Second).UnixNano()
				first, err := m.Evaluator().Evaluate(t.Context(), deadline)
				require.NoError(t, err)
				require.True(t, first.Allow)
				<-time.After(2 * time.Second)
				second, err := m.Evaluator().Evaluate(t.Context(), deadline)
				require.NoError(t, err)
				deterministic := query == "data.t.constant"
				require.Equal(t, deterministic, second.Allow)
				hits := metricstest.GetCounterValue(t, collector, "test_auth_opa_decision_cache_lookups_total", "result", "hit")
				require.Equal(t, deterministic, hits == 1)
				require.NoError(t, m.Close())

			})
		})
	}
}
