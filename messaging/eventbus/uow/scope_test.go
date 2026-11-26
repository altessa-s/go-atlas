// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package uow_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/messaging/eventbus/uow"
	"github.com/altessa-s/go-atlas/messaging/eventbus/uow/uowtest"
)

type memState struct {
	val int
}

func (m *memState) Snapshot() func() {
	v := m.val
	return func() { m.val = v }
}

// newRunner returns a Runner over a fresh Fake with the Fake as detector.
func newRunner(f *uowtest.Fake, opts ...uow.Option) *uow.Runner {
	return uow.New(f, nil, append([]uow.Option{uow.WithDetector(uowtest.Transaction)}, opts...)...)
}

func TestAfterCommit_RootOrderOnDetachedContext(t *testing.T) {
	t.Parallel()

	f := &uowtest.Fake{}
	r := newRunner(f)
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	var events []string
	err := r.Run(ctx, func(txCtx context.Context) error {
		events = append(events, "body-start")
		require.NoError(t, r.AfterCommit(txCtx, func(hctx context.Context) {
			require.False(t, f.InTransaction(hctx), "hook must not run inside the transaction")
			require.NoError(t, hctx.Err(), "hook must run on a cancellation-detached context")
			events = append(events, "hook-1")
		}))
		require.NoError(t, r.AfterCommit(txCtx, func(context.Context) { events = append(events, "hook-2") }))
		require.NoError(t, r.AfterCommit(txCtx, nil))
		cancel() // the caller walks away after the commit decision
		events = append(events, "body-end")
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, []string{"body-start", "body-end", "hook-1", "hook-2"}, events)
}

func TestRun_RetriesIsolateRegistrations(t *testing.T) {
	t.Parallel()

	t.Run("aborted attempts are discarded", func(t *testing.T) {
		t.Parallel()

		st := &memState{val: 10}
		f := &uowtest.Fake{Attempts: 3, Isolate: []uowtest.Snapshotter{st}}
		r := newRunner(f)

		var hooks, effects []int
		err := r.Run(t.Context(), func(txCtx context.Context) error {
			call := f.Calls()
			st.val += call
			require.NoError(t, uow.OnCommit(txCtx, uow.Effect{Label: "e", Apply: func(context.Context) error {
				effects = append(effects, call)
				return nil
			}}))
			return r.AfterCommit(txCtx, func(context.Context) { hooks = append(hooks, call) })
		})
		require.NoError(t, err)
		require.Equal(t, 3, f.Calls())
		require.Equal(t, []int{3}, hooks)
		require.Equal(t, []int{3}, effects)
		require.Equal(t, 13, st.val)
	})

	t.Run("commit retries keep the single attempt", func(t *testing.T) {
		t.Parallel()

		f := &uowtest.Fake{CommitRetries: 2}
		r := newRunner(f)

		var hookRuns int
		err := r.Run(t.Context(), func(txCtx context.Context) error {
			return r.AfterCommit(txCtx, func(context.Context) { hookRuns++ })
		})
		require.NoError(t, err)
		require.Equal(t, 1, f.Calls())
		require.Equal(t, 3, f.CommitAttempts())
		require.Equal(t, 1, hookRuns)
	})

	t.Run("a retained attempt context is closed", func(t *testing.T) {
		t.Parallel()

		f := &uowtest.Fake{Attempts: 2}
		r := newRunner(f)

		var first context.Context
		err := r.Run(t.Context(), func(txCtx context.Context) error {
			if f.Calls() == 1 {
				first = txCtx
				return nil
			}
			require.ErrorIs(t, r.Run(first, func(context.Context) error { return nil }), uow.ErrScopeClosed)
			require.ErrorIs(t, r.AfterCommit(first, func(context.Context) {}), uow.ErrScopeClosed)
			require.ErrorIs(t, uow.OnCommit(first, uow.Effect{Label: "late"}), uow.ErrScopeClosed)
			return nil
		})
		require.NoError(t, err)
	})
}

func TestRun_OutcomeClassification(t *testing.T) {
	t.Parallel()

	errBody := errors.New("body error")
	errCommit := errors.New("commit failed")
	failFirst := func(txn int) error {
		if txn == 1 {
			return errCommit
		}
		return nil
	}

	tests := []struct {
		name        string
		fake        *uowtest.Fake
		body        func(f *uowtest.Fake) error
		wantErr     error
		wantUnknown bool
		wantVal     int
	}{
		{
			name:    "body error is returned as is",
			fake:    &uowtest.Fake{},
			body:    func(*uowtest.Fake) error { return errBody },
			wantErr: errBody,
			wantVal: 1,
		},
		{
			name:        "commit failure after a successful body is unknown",
			fake:        &uowtest.Fake{Fail: errCommit},
			wantErr:     errCommit,
			wantUnknown: true,
			wantVal:     1,
		},
		{
			name:        "lost commit reply is unknown and stays committed",
			fake:        &uowtest.Fake{Unknown: failFirst},
			wantErr:     errCommit,
			wantUnknown: true,
			wantVal:     2,
		},
		{
			name: "body error on the last attempt is not unknown",
			fake: &uowtest.Fake{Attempts: 2},
			body: func(f *uowtest.Fake) error {
				if f.Calls() == 2 {
					return errBody
				}
				return nil
			},
			wantErr: errBody,
			wantVal: 1,
		},
		{
			name:    "body panic recovered by the committer is not unknown",
			fake:    &uowtest.Fake{RecoverPanics: true},
			body:    func(*uowtest.Fake) error { panic("boom") },
			wantErr: uowtest.ErrPanicked,
			wantVal: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			st := &memState{val: 1}
			tc.fake.Isolate = []uowtest.Snapshotter{st}
			r := newRunner(tc.fake)

			var hooks int
			err := r.Run(t.Context(), func(txCtx context.Context) error {
				st.val = 2
				require.NoError(t, r.AfterCommit(txCtx, func(context.Context) { hooks++ }))
				if tc.body != nil {
					return tc.body(tc.fake)
				}
				return nil
			})
			require.ErrorIs(t, err, tc.wantErr)
			require.Equal(t, tc.wantUnknown, errors.Is(err, uow.ErrUnknownOutcome))
			if tc.wantUnknown {
				uoe, ok := errors.AsType[*uow.UnknownOutcomeError](err)
				require.True(t, ok)
				require.Equal(t, errCommit, uoe.Err)
			}
			require.Equal(t, tc.wantVal, st.val)
			require.Zero(t, hooks, "no hook runs after a failed Run")
		})
	}
}

func TestRun_PropagatedBodyPanicClosesUnit(t *testing.T) {
	t.Parallel()

	f := &uowtest.Fake{}
	r := newRunner(f)

	var retained context.Context
	require.PanicsWithValue(t, "boom", func() {
		_ = r.Run(t.Context(), func(txCtx context.Context) error {
			retained = txCtx
			panic("boom")
		})
	})
	require.ErrorIs(t, r.Run(retained, func(context.Context) error { return nil }), uow.ErrScopeClosed)
	require.ErrorIs(t, r.AfterCommit(retained, func(context.Context) {}), uow.ErrScopeClosed)
}

func TestRun_CommitterNeverCallsBody(t *testing.T) {
	t.Parallel()

	errStart := errors.New("start session")
	r := uow.New(committerFunc(func(context.Context, func(context.Context) error) error { return errStart }), nil)
	err := r.Run(t.Context(), func(context.Context) error { return nil })
	require.ErrorIs(t, err, errStart)
	require.NotErrorIs(t, err, uow.ErrUnknownOutcome)
}

type committerFunc func(ctx context.Context, fn func(context.Context) error) error

func (c committerFunc) WithTransaction(ctx context.Context, fn func(context.Context) error) error {
	return c(ctx, fn)
}

func TestRun_Nesting(t *testing.T) {
	t.Parallel()

	errAbort := errors.New("abort root")

	t.Run("nested registrations apply after the root commit", func(t *testing.T) {
		t.Parallel()

		f := &uowtest.Fake{}
		r := newRunner(f)

		var order []string
		err := r.Run(t.Context(), func(txCtx context.Context) error {
			order = append(order, "outer-before")
			require.NoError(t, r.AfterCommit(txCtx, func(context.Context) { order = append(order, "hook-outer-1") }))
			require.NoError(t, r.Run(txCtx, func(nctx context.Context) error {
				order = append(order, "inner")
				require.NoError(t, uow.OnCommit(nctx, uow.Effect{Label: "inner", Apply: func(context.Context) error {
					order = append(order, "effect-inner")
					return nil
				}}))
				return r.AfterCommit(nctx, func(context.Context) { order = append(order, "hook-inner") })
			}))
			order = append(order, "outer-after")
			return r.AfterCommit(txCtx, func(context.Context) { order = append(order, "hook-outer-2") })
		})
		require.NoError(t, err)
		require.Equal(t, 1, f.Txns())
		require.Equal(t, 1, f.Calls())
		require.Equal(t, []string{
			"outer-before", "inner", "outer-after", "hook-outer-1", "effect-inner", "hook-inner", "hook-outer-2",
		}, order)
	})

	t.Run("nested registrations are dropped on root rollback", func(t *testing.T) {
		t.Parallel()

		f := &uowtest.Fake{}
		r := newRunner(f)

		var ran int
		err := r.Run(t.Context(), func(txCtx context.Context) error {
			require.NoError(t, r.Run(txCtx, func(nctx context.Context) error {
				require.NoError(t, uow.OnCommit(nctx, uow.Effect{Label: "e", Apply: func(context.Context) error {
					ran++
					return nil
				}}))
				return r.AfterCommit(nctx, func(context.Context) { ran++ })
			}))
			return errAbort
		})
		require.ErrorIs(t, err, errAbort)
		require.Zero(t, ran)
	})

	t.Run("nested error is returned as is and the root decides", func(t *testing.T) {
		t.Parallel()

		r := newRunner(&uowtest.Fake{})
		err := r.Run(t.Context(), func(txCtx context.Context) error {
			nerr := r.Run(txCtx, func(context.Context) error { return errAbort })
			require.ErrorIs(t, nerr, errAbort)
			require.NotErrorIs(t, nerr, uow.ErrUnknownOutcome)
			return nil
		})
		require.NoError(t, err)
	})

	t.Run("nesting without a detector joins the root", func(t *testing.T) {
		t.Parallel()

		f := &uowtest.Fake{}
		r := uow.New(committerFunc(f.WithTransaction), nil)

		var applied bool
		err := r.Run(t.Context(), func(txCtx context.Context) error {
			require.NoError(t, r.Run(txCtx, func(nctx context.Context) error {
				return uow.OnCommit(nctx, uow.Effect{Label: "e", Apply: func(context.Context) error {
					applied = true
					return nil
				}})
			}))
			require.False(t, applied, "a nested effect must wait for the root commit")
			return nil
		})
		require.NoError(t, err)
		require.True(t, applied)
		require.Equal(t, 1, f.Txns())
	})
}

func TestRun_ForeignTransaction(t *testing.T) {
	t.Parallel()

	t.Run("transaction the runner did not start", func(t *testing.T) {
		t.Parallel()

		f := &uowtest.Fake{}
		r := newRunner(f, uow.WithRunHooksOutsideTransaction())

		require.NoError(t, f.WithTransaction(t.Context(), func(txCtx context.Context) error {
			called := false
			require.ErrorIs(t, r.Run(txCtx, func(context.Context) error {
				called = true
				return nil
			}), uow.ErrForeignTransaction)
			require.ErrorIs(t, r.AfterCommit(txCtx, func(context.Context) { called = true }), uow.ErrForeignTransaction)
			require.False(t, called)
			return nil
		}))
	})

	t.Run("open unit of another runner", func(t *testing.T) {
		t.Parallel()

		fA, fB := &uowtest.Fake{}, &uowtest.Fake{}
		rA, rB := newRunner(fA), newRunner(fB)

		require.NoError(t, rA.Run(t.Context(), func(txCtx context.Context) error {
			called := false
			require.ErrorIs(t, rB.Run(txCtx, func(context.Context) error {
				called = true
				return nil
			}), uow.ErrForeignTransaction)
			require.ErrorIs(t, rB.AfterCommit(txCtx, func(context.Context) { called = true }), uow.ErrForeignTransaction)
			require.False(t, called)
			return nil
		}))
	})

	t.Run("own unit without the transaction", func(t *testing.T) {
		t.Parallel()

		f := &uowtest.Fake{}
		active := true
		r := uow.New(f, nil, uow.WithDetector(func(ctx context.Context) (any, bool) {
			tx, ok := uowtest.Transaction(ctx)
			return tx, active && ok
		}))

		require.NoError(t, r.Run(t.Context(), func(txCtx context.Context) error {
			active = false
			defer func() { active = true }()
			require.ErrorIs(t, r.Run(txCtx, func(context.Context) error { return nil }), uow.ErrForeignTransaction)
			require.ErrorIs(t, r.AfterCommit(txCtx, func(context.Context) {}), uow.ErrForeignTransaction)
			return nil
		}))
	})
}

func TestRun_RetainedContextAfterReturnIsClosed(t *testing.T) {
	t.Parallel()

	r := newRunner(&uowtest.Fake{})
	var retained context.Context
	require.NoError(t, r.Run(t.Context(), func(txCtx context.Context) error {
		retained = txCtx
		return nil
	}))
	require.ErrorIs(t, r.Run(retained, func(context.Context) error { return nil }), uow.ErrScopeClosed)
	require.ErrorIs(t, r.AfterCommit(retained, func(context.Context) {}), uow.ErrScopeClosed)
	require.ErrorIs(t, uow.OnCommit(retained, uow.Effect{Label: "late"}), uow.ErrScopeClosed)
}

func TestAfterCommit_OutsideTransaction(t *testing.T) {
	t.Parallel()

	t.Run("runs at once when enabled", func(t *testing.T) {
		t.Parallel()

		r := newRunner(&uowtest.Fake{}, uow.WithRunHooksOutsideTransaction())
		var ran int
		require.NoError(t, r.AfterCommit(t.Context(), func(context.Context) { ran++ }))
		require.NoError(t, r.AfterCommit(t.Context(), nil))
		require.Equal(t, 1, ran)
	})

	t.Run("no unit of work by default", func(t *testing.T) {
		t.Parallel()

		r := newRunner(&uowtest.Fake{})
		var ran int
		require.ErrorIs(t, r.AfterCommit(t.Context(), func(context.Context) { ran++ }), uow.ErrNoUnitOfWork)
		require.Zero(t, ran)
	})
}

func TestAfterCommit_PanickingHookIsLoggedAndNextRuns(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	r := uow.New(&uowtest.Fake{}, slog.New(slog.NewTextHandler(&buf, nil)))

	var ran []int
	err := r.Run(t.Context(), func(txCtx context.Context) error {
		require.NoError(t, r.AfterCommit(txCtx, func(context.Context) {
			ran = append(ran, 1)
			panic("hook exploded")
		}))
		return r.AfterCommit(txCtx, func(context.Context) { ran = append(ran, 2) })
	})
	require.NoError(t, err)
	require.Equal(t, []int{1, 2}, ran)
	require.Contains(t, buf.String(), "after-commit hook panicked")
}

func TestApply_EffectFailureStillRunsHooks(t *testing.T) {
	t.Parallel()

	applyErr := errors.New("apply b failed")
	rec := &recorder{}
	r := uow.New(&uowtest.Fake{}, nil)

	err := r.Run(t.Context(), func(txCtx context.Context) error {
		require.NoError(t, uow.OnCommit(txCtx, rec.effect("a", nil, nil)))
		require.NoError(t, r.AfterCommit(txCtx, func(context.Context) { rec.events = append(rec.events, "hook:1") }))
		require.NoError(t, uow.OnCommit(txCtx, rec.effect("b", applyErr, nil)))
		require.NoError(t, uow.OnCommit(txCtx, rec.effect("c", nil, nil)))
		return r.AfterCommit(txCtx, func(context.Context) { rec.events = append(rec.events, "hook:2") })
	})
	require.ErrorIs(t, err, applyErr)
	require.NotErrorIs(t, err, uow.ErrUnknownOutcome)
	require.Equal(t, []string{"apply:a", "hook:1", "apply:b", "comp:a", "hook:2"}, rec.events)
}

func TestAfterCommit_ConcurrentRegistration(t *testing.T) {
	t.Parallel()

	r := newRunner(&uowtest.Fake{})
	const n = 64
	var (
		mu  sync.Mutex
		ran int
	)
	err := r.Run(t.Context(), func(txCtx context.Context) error {
		var wg sync.WaitGroup
		for range n {
			wg.Go(func() {
				require.NoError(t, r.AfterCommit(txCtx, func(context.Context) {
					mu.Lock()
					ran++
					mu.Unlock()
				}))
			})
		}
		wg.Wait()
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, n, ran)
}

func TestUnknownOutcomeError(t *testing.T) {
	t.Parallel()

	cause := errors.New("reply lost")
	tests := []struct {
		name    string
		err     *uow.UnknownOutcomeError
		wantMsg string
	}{
		{name: "nil receiver", err: nil, wantMsg: uow.ErrUnknownOutcome.Error()},
		{name: "nil cause", err: &uow.UnknownOutcomeError{}, wantMsg: uow.ErrUnknownOutcome.Error()},
		{name: "with cause", err: &uow.UnknownOutcomeError{Err: cause}, wantMsg: uow.ErrUnknownOutcome.Error() + ": reply lost"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.wantMsg, tc.err.Error())
		})
	}
	require.ErrorIs(t, &uow.UnknownOutcomeError{}, uow.ErrUnknownOutcome)
	require.ErrorIs(t, &uow.UnknownOutcomeError{Err: cause}, cause)
	require.Nil(t, (*uow.UnknownOutcomeError)(nil).Unwrap())
}

// TestRun_IndependentInnerTransactionIsForeign: review finding R26-1 — a unit only checked that some transaction was running, so
// inside runner A's transaction a context of an independent transaction of another client (B, started inside A's body) joined A:
// A.AfterCommit and OnCommit registered on A, and A.Run nested on A, with B's context; when B aborted and its error was handled, A
// committed and ran the hook for B's aborted writes (verified on 96a1b191: the hook ran). A unit is bound to the transaction its
// attempt runs in, and joining or registering checks that ctx is inside that very transaction.
func TestRun_IndependentInnerTransactionIsForeign(t *testing.T) {
	t.Parallel()

	outer, inner := &uowtest.Fake{}, &uowtest.Fake{}
	r := newRunner(outer)
	errAbortInner := errors.New("inner aborted")
	var hooks, effects atomic.Int32

	require.NoError(t, r.Run(t.Context(), func(txCtx context.Context) error {
		err := inner.WithTransaction(txCtx, func(innerCtx context.Context) error {
			require.ErrorIs(t, r.AfterCommit(innerCtx, func(context.Context) { hooks.Add(1) }), uow.ErrForeignTransaction)
			require.ErrorIs(t, uow.OnCommit(innerCtx, uow.Effect{Label: "inner", Apply: func(context.Context) error {
				effects.Add(1)
				return nil
			}}), uow.ErrForeignTransaction)
			require.ErrorIs(t, r.Run(innerCtx, func(context.Context) error { return nil }), uow.ErrForeignTransaction)
			return errAbortInner
		})
		require.ErrorIs(t, err, errAbortInner) // handled: the outer transaction goes on and commits

		// The outer transaction's own context still joins and registers.
		require.NoError(t, r.AfterCommit(txCtx, func(context.Context) { hooks.Add(10) }))
		require.NoError(t, r.Run(txCtx, func(nested context.Context) error {
			return uow.OnCommit(nested, uow.Effect{Label: "outer", Apply: func(context.Context) error {
				effects.Add(10)
				return nil
			}})
		}))
		return nil
	}))
	require.Equal(t, int32(10), hooks.Load(), "only the outer transaction's hook runs")
	require.Equal(t, int32(10), effects.Load(), "only the outer transaction's effect applies")
}

// TestFake_InnermostTransaction checks the detector and joins of uowtest against store/mongo's semantics: the innermost attempt
// identifies the transaction, a Fake joins only its own innermost attempt, and another Fake's attempt inside it is another
// transaction.
func TestFake_InnermostTransaction(t *testing.T) {
	t.Parallel()

	a, b := &uowtest.Fake{}, &uowtest.Fake{}
	_, ok := uowtest.Transaction(t.Context())
	require.False(t, ok)
	require.NoError(t, a.WithTransaction(t.Context(), func(aCtx context.Context) error {
		txA, okA := uowtest.Transaction(aCtx)
		require.True(t, okA)
		require.True(t, a.InTransaction(aCtx))
		require.NoError(t, a.WithTransaction(aCtx, func(joined context.Context) error {
			tx, _ := uowtest.Transaction(joined)
			require.True(t, tx == txA, "a joined call is the same transaction") // identities compare with ==, as uow does
			return nil
		}))
		return b.WithTransaction(aCtx, func(bCtx context.Context) error {
			txB, okB := uowtest.Transaction(bCtx)
			require.True(t, okB)
			require.True(t, txA != txB, "another Fake's attempt is another transaction")
			require.False(t, a.InTransaction(bCtx), "a does not join b's transaction")
			require.True(t, b.InTransaction(bCtx))
			return nil
		})
	}))
	require.Equal(t, 1, a.Txns())
	require.Equal(t, 1, b.Txns())
}

func TestRun_CommitterConfirmsItsBody(t *testing.T) {
	t.Parallel()
	observerErr := errors.New("observer failed after commit")
	r := uow.New(committerFunc(func(ctx context.Context, fn func(context.Context) error) error {
		tx, confirm := uow.WithCommitConfirmation(ctx)
		require.NotNil(t, confirm)
		if err := fn(tx); err != nil {
			return err
		}
		confirm()
		return observerErr
	}), nil)
	var applied bool
	err := r.Run(t.Context(), func(ctx context.Context) error {
		return r.AfterCommit(ctx, func(context.Context) { applied = true })
	})
	require.ErrorIs(t, err, observerErr)
	require.NotErrorIs(t, err, uow.ErrUnknownOutcome)
	require.True(t, applied)
}

type classifiedCommitter struct {
	committerFunc
	uow.OutcomeClassifier
}

type definitiveAbort struct{}

func (definitiveAbort) CommitOutcomeUnknown(error) bool { return false }

func TestOutcomeClassifierDefinitiveAbort(t *testing.T) {
	t.Parallel()
	cause := errors.New("rolled back")
	c := classifiedCommitter{committerFunc: func(ctx context.Context, body func(context.Context) error) error {
		if err := body(ctx); err != nil {
			return err
		}
		return cause
	}, OutcomeClassifier: definitiveAbort{}}
	err := uow.New(c, nil).Run(t.Context(), func(context.Context) error { return nil })
	require.ErrorIs(t, err, cause)
	require.NotErrorIs(t, err, uow.ErrUnknownOutcome)
}

func TestApply_CallbackPanicRedactsLogs(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	previous := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(previous) })

	const marker = "private-callback-payload"
	r := uow.New(&uowtest.Fake{}, logger)
	err := r.Run(t.Context(), func(ctx context.Context) error {
		require.NoError(t, r.AfterCommit(ctx, func(context.Context) { panic(marker + "-hook") }))
		require.NoError(t, uow.OnCommit(ctx, uow.Effect{
			Apply:      func(context.Context) error { return nil },
			Compensate: func(context.Context) error { panic(marker + "-compensate") },
		}))
		return uow.OnCommit(ctx, uow.Effect{
			Apply: func(context.Context) error { panic(marker + "-apply") },
		})
	})
	require.ErrorIs(t, err, uow.ErrCallbackPanicked)
	require.NotContains(t, err.Error(), marker)
	require.NotContains(t, logs.String(), marker)
	require.Equal(t, 3, bytes.Count(logs.Bytes(), []byte("stacktrace=")))
	require.Contains(t, logs.String(), "TestApply_CallbackPanicRedactsLogs")
}

func TestApply_EffectPanicStillCompensatesAndRunsHooks(t *testing.T) {
	t.Parallel()
	rec := &recorder{}
	r := uow.New(&uowtest.Fake{}, nil)
	var runErr error
	func() {
		defer func() { _ = recover() }()
		runErr = r.Run(t.Context(), func(ctx context.Context) error {
			require.NoError(t, uow.OnCommit(ctx, rec.effect("a", nil, nil)))
			require.NoError(t, uow.OnCommit(ctx, uow.Effect{Label: "b",
				Apply: func(context.Context) error { panic("apply") },
			}))
			return r.AfterCommit(ctx, func(context.Context) { rec.events = append(rec.events, "hook") })
		})
	}()
	require.Equal(t, []string{"apply:a", "comp:a", "hook"}, rec.events)
	require.Error(t, runErr)
}

func TestApply_CompensationPanicStillCompensatesAndRunsHooks(t *testing.T) {
	t.Parallel()
	rec := &recorder{}
	r := uow.New(&uowtest.Fake{}, nil)
	applyErr := errors.New("apply failed")
	var runErr error
	func() {
		defer func() { _ = recover() }()
		runErr = r.Run(t.Context(), func(ctx context.Context) error {
			require.NoError(t, uow.OnCommit(ctx, rec.effect("a", nil, nil)))
			effect := rec.effect("b", nil, nil)
			effect.Compensate = func(context.Context) error { panic("compensation") }
			require.NoError(t, uow.OnCommit(ctx, effect))
			require.NoError(t, uow.OnCommit(ctx, rec.effect("c", applyErr, nil)))
			return r.AfterCommit(ctx, func(context.Context) { rec.events = append(rec.events, "hook") })
		})
	}()
	require.Equal(t, []string{"apply:a", "apply:b", "apply:c", "comp:a", "hook"}, rec.events)
	require.ErrorIs(t, runErr, applyErr)
	require.ErrorContains(t, runErr, "compensate post-commit effect")
}
