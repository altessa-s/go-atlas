// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package uow_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/eventbus/uow"
)

// fakeCommitter invokes fn attempts times (default 1, to simulate a driver
// retry) and then returns commitErr.
type fakeCommitter struct {
	attempts  int
	commitErr error
}

func (f *fakeCommitter) WithTransaction(ctx context.Context, fn func(context.Context) error) error {
	n := f.attempts
	if n == 0 {
		n = 1
	}
	for range n {
		if err := fn(ctx); err != nil {
			return err
		}
	}
	return f.commitErr
}

// recorder collects effect labels in the order their Apply/Compensate runs.
type recorder struct{ events []string }

func (r *recorder) effect(label string, applyErr, compErr error) uow.Effect {
	return uow.Effect{
		Label: label,
		Apply: func(context.Context) error {
			r.events = append(r.events, "apply:"+label)
			return applyErr
		},
		Compensate: func(context.Context) error {
			r.events = append(r.events, "comp:"+label)
			return compErr
		},
	}
}

func TestRun_HappyPathAppliesInOrderNoCompensation(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	r := uow.New(&fakeCommitter{}, nil)
	err := r.Run(t.Context(), func(ctx context.Context) error {
		require.NoError(t, uow.OnCommit(ctx, rec.effect("a", nil, nil)))
		require.NoError(t, uow.OnCommit(ctx, rec.effect("b", nil, nil)))
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, []string{"apply:a", "apply:b"}, rec.events)
}

func TestRun_BodyErrorAppliesNothing(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	sentinel := errors.New("body failed")
	r := uow.New(&fakeCommitter{}, nil)
	err := r.Run(t.Context(), func(ctx context.Context) error {
		require.NoError(t, uow.OnCommit(ctx, rec.effect("a", nil, nil)))
		return sentinel
	})
	require.ErrorIs(t, err, sentinel)
	require.Empty(t, rec.events)
}

func TestRun_CommitFailureAppliesNothing(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	commitErr := errors.New("commit failed")
	r := uow.New(&fakeCommitter{commitErr: commitErr}, nil)
	err := r.Run(t.Context(), func(ctx context.Context) error {
		require.NoError(t, uow.OnCommit(ctx, rec.effect("a", nil, nil)))
		return nil
	})
	require.ErrorIs(t, err, commitErr)
	require.Empty(t, rec.events)
}

func TestRun_ApplyFailureCompensatesAppliedInLIFO(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	applyErr := errors.New("apply c failed")
	r := uow.New(&fakeCommitter{}, nil)
	err := r.Run(t.Context(), func(ctx context.Context) error {
		require.NoError(t, uow.OnCommit(ctx, rec.effect("a", nil, nil)))
		require.NoError(t, uow.OnCommit(ctx, rec.effect("b", nil, nil)))
		require.NoError(t, uow.OnCommit(ctx, rec.effect("c", applyErr, nil)))
		require.NoError(t, uow.OnCommit(ctx, rec.effect("d", nil, nil)))
		return nil
	})
	require.ErrorIs(t, err, applyErr)
	// a, b apply; c's Apply fails; d never runs; compensate b then a (LIFO).
	require.Equal(t, []string{"apply:a", "apply:b", "apply:c", "comp:b", "comp:a"}, rec.events)
}

func TestRun_RetryAppliesEachEffectOnce(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	r := uow.New(&fakeCommitter{attempts: 2}, nil)
	err := r.Run(t.Context(), func(ctx context.Context) error {
		require.NoError(t, uow.OnCommit(ctx, rec.effect("a", nil, nil)))
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, []string{"apply:a"}, rec.events, "the per-attempt reset must dedupe effects")
}

func TestRun_NilCompensateSkippedWithoutPanic(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	applyErr := errors.New("apply b failed")
	r := uow.New(&fakeCommitter{}, nil)
	err := r.Run(t.Context(), func(ctx context.Context) error {
		// "a" is best-effort: nil Compensate.
		require.NoError(t, uow.OnCommit(ctx, uow.Effect{Label: "a", Apply: func(context.Context) error {
			rec.events = append(rec.events, "apply:a")
			return nil
		}}))
		require.NoError(t, uow.OnCommit(ctx, rec.effect("b", applyErr, nil)))
		return nil
	})
	require.ErrorIs(t, err, applyErr)
	// a applied (nil Compensate, skipped); b's Apply fails; nothing compensated.
	require.Equal(t, []string{"apply:a", "apply:b"}, rec.events)
}

func TestRun_CompensationFailureJoinsBothCauses(t *testing.T) {
	t.Parallel()

	applyErr := errors.New("apply b failed")
	compErr := errors.New("compensate a failed")
	r := uow.New(&fakeCommitter{}, nil)
	err := r.Run(t.Context(), func(ctx context.Context) error {
		require.NoError(t, uow.OnCommit(ctx, uow.Effect{
			Label:      "a",
			Apply:      func(context.Context) error { return nil },
			Compensate: func(context.Context) error { return compErr },
		}))
		require.NoError(t, uow.OnCommit(ctx, uow.Effect{
			Label: "b",
			Apply: func(context.Context) error { return applyErr },
		}))
		return nil
	})
	require.ErrorIs(t, err, applyErr)
	require.ErrorIs(t, err, compErr)
}

func TestOnCommit_NoUnitOfWork(t *testing.T) {
	t.Parallel()

	err := uow.OnCommit(t.Context(), uow.Effect{Label: "a"})
	require.ErrorIs(t, err, uow.ErrNoUnitOfWork)
}

func TestRun_CompensationRunsOnCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	rec := &recorder{}
	r := uow.New(&fakeCommitter{}, nil)
	err := r.Run(ctx, func(innerCtx context.Context) error {
		require.NoError(t, uow.OnCommit(innerCtx, uow.Effect{
			Label: "a",
			Apply: func(context.Context) error {
				cancel() // the caller walks away after the first effect applies
				rec.events = append(rec.events, "apply:a")
				return nil
			},
			Compensate: func(cctx context.Context) error {
				require.NoError(t, cctx.Err(), "compensate must run on a non-canceled context")
				rec.events = append(rec.events, "comp:a")
				return nil
			},
		}))
		require.NoError(t, uow.OnCommit(innerCtx, uow.Effect{
			Label: "b",
			Apply: func(actx context.Context) error { return actx.Err() }, // fails: canceled
		}))
		return nil
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, []string{"apply:a", "comp:a"}, rec.events)
}
