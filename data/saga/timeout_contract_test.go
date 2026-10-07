// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package saga_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/saga"
	"github.com/altessa-s/go-atlas/data/saga/storages/memory"
)

type budgetStore struct {
	saga.Storage
	update func(context.Context, *saga.Instance)
	fetch  func(context.Context)
}

func (s budgetStore) Update(ctx context.Context, inst *saga.Instance) error {
	if s.update != nil {
		s.update(ctx, inst)
	}
	return s.Storage.Update(ctx, inst)
}
func (s budgetStore) FetchRecoverable(ctx context.Context, definition string, now time.Time, limit int) ([]*saga.Instance, error) {
	if s.fetch != nil {
		s.fetch(ctx)
	}
	return s.Storage.FetchRecoverable(ctx, definition, now, limit)
}

func TestNestedTimeoutBudgets(t *testing.T) {
	t.Parallel()
	for _, parent := range []time.Duration{0, time.Hour, 50 * time.Millisecond} {
		t.Run(parent.String(), func(t *testing.T) {
			t.Parallel()
			const execution = time.Second
			const step = 100 * time.Millisecond
			const persistence = 10 * time.Millisecond
			ctx := t.Context()
			if parent > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, parent)
				defer cancel()
			}
			var stepDeadline time.Time
			store := budgetStore{Storage: memory.New(), update: func(ctx context.Context, inst *saga.Instance) {
				deadline, ok := ctx.Deadline()
				require.True(t, ok)
				require.LessOrEqual(t, time.Until(deadline), persistence)
			}}
			def := saga.NewDefinition[struct{}]("budgets").Step("step", func(ctx context.Context, _ *struct{}) error {
				stepDeadline, _ = ctx.Deadline()
				return nil
			}).ReadOnly().MustBuild()
			o := saga.New(store, def, saga.WithExecutionTimeout(execution), saga.WithStepTimeout(step), saga.WithStorageTimeout(persistence))
			_, err := o.Start(ctx, "one", struct{}{})
			require.NoError(t, err)
			require.False(t, stepDeadline.IsZero())
			require.LessOrEqual(t, time.Until(stepDeadline), step)
			if parent > 0 {
				deadline, _ := ctx.Deadline()
				require.False(t, stepDeadline.After(deadline))
			}
		})
	}
}

func TestRecoveryStorageTimeoutCapsCycle(t *testing.T) {
	t.Parallel()
	const persistence = 10 * time.Millisecond
	called := false
	store := budgetStore{Storage: memory.New(), fetch: func(ctx context.Context) {
		called = true
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.LessOrEqual(t, time.Until(deadline), persistence)
	}}
	def := saga.NewDefinition[struct{}]("recovery-budget").Step("noop", func(context.Context, *struct{}) error { return nil }).ReadOnly().MustBuild()
	o := saga.New(store, def, saga.WithRecoveryTimeout(time.Second), saga.WithStorageTimeout(persistence))
	ctx, cancel := context.WithTimeout(t.Context(), time.Hour)
	defer cancel()
	require.NoError(t, o.RunRecoveryCycle(ctx))
	require.True(t, called)
}
