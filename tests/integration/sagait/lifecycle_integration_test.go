// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sagait_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/reliability/saga"
)

var errInjected = errors.New("injected store failure")

// failFirstUpdate fails the first Update once, then delegates.
type failFirstUpdate struct {
	saga.Storage
	failed atomic.Bool
}

func (s *failFirstUpdate) Update(ctx context.Context, inst *saga.Instance) error {
	if s.failed.CompareAndSwap(false, true) {
		return errInjected
	}
	return s.Storage.Update(ctx, inst)
}

// lifecycleBackends lists every live backend with its store constructor.
func lifecycleBackends() map[string]func(testing.TB) saga.Storage {
	backends := map[string]func(testing.TB) saga.Storage{"mongodb": newMongoStore, "redis": newRedisStore, "redis-cluster": newRedisClusterStore}
	for _, target := range sqlTargets() {
		backends[target.name] = target.store
	}
	return backends
}

func countingDef(name string, runs *atomic.Int32) *saga.Definition[order] {
	return saga.NewDefinition[order](name).
		Step("a", func(context.Context, *order) error { runs.Add(1); return nil }).ReadOnly().
		MustBuild()
}

// TestStartWithFailedFirstWriteIsRecovered pins RL-T4: a Start whose first
// write after Create fails leaves an instance recovery finds and completes.
func TestStartWithFailedFirstWriteIsRecovered(t *testing.T) {
	t.Parallel()
	for name, newStore := range lifecycleBackends() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var runs atomic.Int32
			store := &failFirstUpdate{Storage: newStore(t)}
			orch := saga.New(store, countingDef("lease", &runs))

			_, err := orch.Start(t.Context(), "x", order{})
			require.ErrorIs(t, err, errInjected)
			require.NoError(t, orch.RunRecoveryCycle(t.Context()))

			inst, err := store.Get(t.Context(), "x")
			require.NoError(t, err)
			require.Equal(t, saga.StatusCompleted, inst.Status)
			require.EqualValues(t, 1, runs.Load())
		})
	}
}

// TestRecoveryQuarantinesPoisonInstance pins RL-T30: an undecodable instance
// is failed and dead-lettered, and the instance behind it still recovers.
func TestRecoveryQuarantinesPoisonInstance(t *testing.T) {
	t.Parallel()
	for name, newStore := range lifecycleBackends() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			store := newStore(t)
			def := saga.NewDefinition[order]("recover").
				Step("a", func(context.Context, *order) error { return nil }).Compensate(func(context.Context, *order) error { return nil }).
				MustBuild()
			require.NoError(t, store.Create(t.Context(), &saga.Instance{
				ID: "a-poison", Definition: "recover", Status: saga.StatusCompensating, Stage: 1, Data: []byte("{not json"),
			}))
			require.NoError(t, store.Create(t.Context(), &saga.Instance{
				ID: "b-valid", Definition: "recover", Status: saga.StatusCompensating, Stage: 1, Data: []byte("{}"),
			}))

			var dead atomic.Int32
			orch := saga.New(store, def, saga.WithRecoveryBatchSize(1), saga.WithOnDeadLetter(func(*saga.Instance) { dead.Add(1) }))
			for range 3 {
				_ = orch.RunRecoveryCycle(t.Context())
			}

			valid, err := store.Get(t.Context(), "b-valid")
			require.NoError(t, err)
			require.Equal(t, saga.StatusCompensated, valid.Status)
			poison, err := store.Get(t.Context(), "a-poison")
			require.NoError(t, err)
			require.Equal(t, saga.StatusFailed, poison.Status)
			require.EqualValues(t, 1, dead.Load())
		})
	}
}

// TestRetentionCycleDeletesTerminalInstances pins RL-T11 end to end: finished
// sagas older than the window are deleted, live ones are not.
func TestRetentionCycleDeletesTerminalInstances(t *testing.T) {
	t.Parallel()
	for name, newStore := range lifecycleBackends() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var runs atomic.Int32
			store := newStore(t)
			orch := saga.New(store, countingDef("retain", &runs), saga.WithRetention(time.Hour), saga.WithRetentionBatchSize(2))
			for _, id := range []string{"a", "b", "c"} {
				_, err := orch.Start(t.Context(), id, order{})
				require.NoError(t, err)
			}
			require.NoError(t, store.Create(t.Context(), &saga.Instance{
				ID: "live", Definition: "retain", Status: saga.StatusCompensating, UpdatedAt: time.Now(),
			}))
			// A finished saga of another definition in the same store is never touched.
			require.NoError(t, store.Create(t.Context(), &saga.Instance{
				ID: "other", Definition: "other-saga", Status: saga.StatusCompleted, UpdatedAt: time.Now().Add(-3 * time.Hour),
			}))

			// Age the finished sagas past the window.
			for _, id := range []string{"a", "b", "c"} {
				inst, err := store.Get(t.Context(), id)
				require.NoError(t, err)
				inst.UpdatedAt = time.Now().Add(-2 * time.Hour)
				require.NoError(t, store.Update(t.Context(), inst))
			}
			require.NoError(t, orch.RunRetentionCycle(t.Context()))

			for _, id := range []string{"a", "b", "c"} {
				_, err := store.Get(t.Context(), id)
				require.ErrorIs(t, err, saga.ErrInstanceNotFound, id)
			}
			_, err := store.Get(t.Context(), "live")
			require.NoError(t, err)
			_, err = store.Get(t.Context(), "other")
			require.NoError(t, err, "another definition's history must survive")

			// The id is free again: Start runs the saga anew (documented).
			runs.Store(0)
			_, err = orch.Start(t.Context(), "a", order{})
			require.NoError(t, err)
			require.EqualValues(t, 1, runs.Load())
		})
	}
}
