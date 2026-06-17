// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package saga_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/saga"
	"github.com/altessa-s/go-atlas/data/saga/storages/memory"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	sagaerrs "github.com/altessa-s/go-atlas/data/saga/errs"
)

var (
	errBoom      = errors.New("boom")
	errPermanent = errors.New("permanent")
)

// order is the shared saga data type used across the tests.
type order struct {
	Applied []string `json:"applied"`
}

// recorder is a concurrency-safe ordered event log shared by step closures.
type recorder struct {
	mu     sync.Mutex
	events []string
}

func (r *recorder) add(s string) {
	r.mu.Lock()
	r.events = append(r.events, s)
	r.mu.Unlock()
}

func (r *recorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.events)
}

func (r *recorder) count(s string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, e := range r.events {
		if e == s {
			n++
		}
	}
	return n
}

// doStep returns a forward action that records "do:name", optionally fails with
// failErr, and otherwise appends name to the order's Applied slice.
func doStep(r *recorder, name string, failErr error) saga.StepFunc[order] {
	return func(_ context.Context, d *order) error {
		r.add("do:" + name)
		if failErr != nil {
			return failErr
		}
		d.Applied = append(d.Applied, name)
		return nil
	}
}

// undoStep returns a compensation that records "undo:name".
func undoStep(r *recorder, name string) saga.CompensateFunc[order] {
	return func(_ context.Context, _ *order) error {
		r.add("undo:" + name)
		return nil
	}
}

// tapStep returns a forward action that only records "do:name" and does not
// mutate the shared data. Use it for parallel-group members, whose actions run
// concurrently and so must not write overlapping fields of *order.
func tapStep(r *recorder, name string) saga.StepFunc[order] {
	return func(_ context.Context, _ *order) error {
		r.add("do:" + name)
		return nil
	}
}

// fastOpts keeps retry backoff negligible so tests run quickly.
func fastOpts(extra ...saga.Option) []saga.Option {
	base := []saga.Option{
		saga.WithStepRetryBaseDelay(time.Millisecond),
		saga.WithStepRetryMaxDelay(time.Millisecond),
	}
	return append(base, extra...)
}

func TestStartHappyPath(t *testing.T) {
	t.Parallel()
	r := &recorder{}
	tc := testhelpers.NewTestCollector()

	def := saga.NewDefinition[order]("happy").
		Step("a", doStep(r, "a", nil)).Compensate(undoStep(r, "a")).
		Step("b", doStep(r, "b", nil)).Compensate(undoStep(r, "b")).
		Step("c", doStep(r, "c", nil)).
		MustBuild()

	orch := saga.New(memory.New(), def, saga.WithCollector(tc))
	inst, err := orch.Start(t.Context(), "o1", order{})
	require.NoError(t, err)
	require.Equal(t, saga.StatusCompleted, inst.Status)
	require.Equal(t, []string{"do:a", "do:b", "do:c"}, r.snapshot())

	require.Equal(t, float64(1), testhelpers.GetCounterValue(t, tc, "test_saga_started_total"))
	require.Equal(t, float64(1), testhelpers.GetCounterValue(t, tc, "test_saga_completed_total"))
}

func TestStartCompensatesInReverse(t *testing.T) {
	t.Parallel()
	r := &recorder{}
	tc := testhelpers.NewTestCollector()

	def := saga.NewDefinition[order]("rollback").
		Step("a", doStep(r, "a", nil)).Compensate(undoStep(r, "a")).
		Step("b", doStep(r, "b", nil)). // read-only: no compensation, must be skipped on rollback
		Step("c", doStep(r, "c", errBoom)).Compensate(undoStep(r, "c")).
		MustBuild(saga.WithCompensationPolicy(saga.PolicyDisabled))

	orch := saga.New(memory.New(), def, fastOpts(saga.WithMaxStepAttempts(1), saga.WithCollector(tc))...)
	inst, err := orch.Start(t.Context(), "o1", order{})

	require.ErrorIs(t, err, errBoom)
	require.Equal(t, saga.StatusCompensated, inst.Status)
	// c failed (no commit, no undo for c), then b skipped (no compensation), then a undone.
	require.Equal(t, []string{"do:a", "do:b", "do:c", "undo:a"}, r.snapshot())
	require.Equal(t, float64(1), testhelpers.GetCounterValue(t, tc, "test_saga_compensated_total"))
}

func TestParallelStageRunsAndCompensatesConcurrently(t *testing.T) {
	t.Parallel()
	r := &recorder{}

	def := saga.NewDefinition[order]("parallel").
		Parallel("notify",
			saga.NewStep("email", tapStep(r, "email")).Compensate(undoStep(r, "email")),
			saga.NewStep("sms", tapStep(r, "sms")).Compensate(undoStep(r, "sms")),
		).
		Step("commit", doStep(r, "commit", errBoom)).Compensate(undoStep(r, "commit")).
		MustBuild()

	orch := saga.New(memory.New(), def, fastOpts(saga.WithMaxStepAttempts(1))...)
	inst, err := orch.Start(t.Context(), "o1", order{})

	require.ErrorIs(t, err, errBoom)
	require.Equal(t, saga.StatusCompensated, inst.Status)

	events := r.snapshot()
	require.Contains(t, events, "do:email")
	require.Contains(t, events, "do:sms")
	require.Contains(t, events, "undo:email")
	require.Contains(t, events, "undo:sms")
	require.NotContains(t, events, "undo:commit") // commit never committed
}

func TestPivotRollsForwardAndDeadLetters(t *testing.T) {
	t.Parallel()
	r := &recorder{}
	tc := testhelpers.NewTestCollector()

	var dead atomic.Pointer[saga.Instance]

	def := saga.NewDefinition[order]("pivot").
		Step("reserve", doStep(r, "reserve", nil)).Compensate(undoStep(r, "reserve")).
		Step("charge", doStep(r, "charge", errBoom)).Compensate(undoStep(r, "charge")).Pivot().
		MustBuild()

	orch := saga.New(memory.New(), def, fastOpts(
		saga.WithMaxStepAttempts(1),
		saga.WithCollector(tc),
		saga.WithOnDeadLetter(func(inst *saga.Instance) { dead.Store(inst) }),
	)...)
	inst, err := orch.Start(t.Context(), "o1", order{})

	require.ErrorIs(t, err, errBoom)
	require.Equal(t, saga.StatusFailed, inst.Status)
	// Post-pivot failure rolls forward then gives up: no compensation of "reserve".
	require.Equal(t, []string{"do:reserve", "do:charge"}, r.snapshot())

	require.NotNil(t, dead.Load())
	require.Equal(t, saga.StatusFailed, dead.Load().Status)
	require.Equal(t, float64(1), testhelpers.GetCounterValue(t, tc, "test_saga_failed_total"))
}

func TestPanicInStepTriggersCompensation(t *testing.T) {
	t.Parallel()
	r := &recorder{}

	def := saga.NewDefinition[order]("panic").
		Step("a", doStep(r, "a", nil)).Compensate(undoStep(r, "a")).
		Step("b", func(_ context.Context, _ *order) error {
			r.add("do:b")
			panic("kaboom")
		}).Compensate(undoStep(r, "b")).
		MustBuild()

	orch := saga.New(memory.New(), def, fastOpts(saga.WithMaxStepAttempts(1))...)
	inst, err := orch.Start(t.Context(), "o1", order{})

	require.Error(t, err)
	require.Equal(t, saga.StatusCompensated, inst.Status)
	require.Equal(t, []string{"do:a", "do:b", "undo:a"}, r.snapshot())
}

func TestRetryThenSucceed(t *testing.T) {
	t.Parallel()
	r := &recorder{}
	var attempts atomic.Int32

	flaky := func(_ context.Context, d *order) error {
		r.add("do:flaky")
		if attempts.Add(1) < 3 {
			return errBoom
		}
		d.Applied = append(d.Applied, "flaky")
		return nil
	}

	def := saga.NewDefinition[order]("retry").
		Step("flaky", flaky).Compensate(undoStep(r, "flaky")).
		MustBuild()

	orch := saga.New(memory.New(), def, fastOpts(saga.WithMaxStepAttempts(3))...)
	inst, err := orch.Start(t.Context(), "o1", order{})

	require.NoError(t, err)
	require.Equal(t, saga.StatusCompleted, inst.Status)
	require.Equal(t, 3, r.count("do:flaky"))
}

func TestShouldRetryShortCircuits(t *testing.T) {
	t.Parallel()
	r := &recorder{}

	def := saga.NewDefinition[order]("noretry").
		Step("a", doStep(r, "a", errPermanent)).Compensate(undoStep(r, "a")).
		MustBuild()

	orch := saga.New(memory.New(), def, fastOpts(
		saga.WithMaxStepAttempts(5),
		saga.WithShouldRetry(func(err error) bool { return !errors.Is(err, errPermanent) }),
	)...)
	inst, err := orch.Start(t.Context(), "o1", order{})

	require.ErrorIs(t, err, errPermanent)
	require.Equal(t, saga.StatusCompensated, inst.Status)
	require.Equal(t, 1, r.count("do:a")) // short-circuited: a single attempt
}

func TestStartIsIdempotentOnID(t *testing.T) {
	t.Parallel()
	r := &recorder{}

	def := saga.NewDefinition[order]("idem").
		Step("a", doStep(r, "a", nil)).
		MustBuild()

	store := memory.New()
	orch := saga.New(store, def)

	inst1, err := orch.Start(t.Context(), "o1", order{})
	require.NoError(t, err)
	require.Equal(t, saga.StatusCompleted, inst1.Status)

	inst2, err := orch.Start(t.Context(), "o1", order{})
	require.ErrorIs(t, err, sagaerrs.ErrAlreadyTerminal)
	require.Equal(t, saga.StatusCompleted, inst2.Status)
	require.Equal(t, 1, r.count("do:a")) // not re-run
}

// flakyStore wraps a Store and fails the configured Nth Update to simulate a
// crash between a step succeeding and its checkpoint being persisted.
type flakyStore struct {
	saga.Store
	failAt  int
	updates int
}

func (f *flakyStore) Update(ctx context.Context, inst *saga.Instance) error {
	f.updates++
	if f.failAt > 0 && f.updates == f.failAt {
		return errBoom
	}
	return f.Store.Update(ctx, inst)
}

func TestResumeAfterCrash(t *testing.T) {
	t.Parallel()
	r := &recorder{}

	build := func() *saga.Definition[order] {
		return saga.NewDefinition[order]("crash").
			Step("a", doStep(r, "a", nil)).Compensate(undoStep(r, "a")).
			Step("b", doStep(r, "b", nil)).Compensate(undoStep(r, "b")).
			Step("c", doStep(r, "c", nil)).
			MustBuild()
	}

	mem := memory.New()
	flaky := &flakyStore{Store: mem, failAt: 2} // fail persisting the checkpoint after stage 1 (b)

	orch1 := saga.New(flaky, build())
	inst, err := orch1.Start(t.Context(), "o1", order{})
	require.ErrorIs(t, err, errBoom)
	require.Equal(t, saga.StatusRunning, inst.Status) // left non-terminal

	// Resume over the healthy underlying store with a fresh orchestrator.
	orch2 := saga.New(mem, build())
	resumed, err := orch2.Resume(t.Context(), "o1")
	require.NoError(t, err)
	require.Equal(t, saga.StatusCompleted, resumed.Status)

	// Stage "b" re-ran on resume (idempotency); "a" and "c" ran once.
	require.Equal(t, 1, r.count("do:a"))
	require.Equal(t, 2, r.count("do:b"))
	require.Equal(t, 1, r.count("do:c"))
}

func TestVersionConflict(t *testing.T) {
	t.Parallel()
	store := memory.New()
	ctx := t.Context()

	inst := &saga.Instance{ID: "v1", Definition: "x", Status: saga.StatusRunning}
	require.NoError(t, store.Create(ctx, inst))

	a, err := store.Get(ctx, "v1")
	require.NoError(t, err)
	b := a.Clone()

	require.NoError(t, store.Update(ctx, a)) // a wins, version bumped
	err = store.Update(ctx, b)               // b carries the stale version
	require.ErrorIs(t, err, sagaerrs.ErrVersionConflict)
}

func TestBuildValidation(t *testing.T) {
	t.Parallel()

	_, err := saga.NewDefinition[order]("empty").Build()
	require.ErrorIs(t, err, sagaerrs.ErrEmptyDefinition)

	_, err = saga.NewDefinition[order]("strict").
		Step("a", doStep(&recorder{}, "a", nil)). // no compensation, not read-only
		Build(saga.WithCompensationPolicy(saga.PolicyEnforce))
	require.ErrorIs(t, err, sagaerrs.ErrNoCompensation)

	require.Panics(t, func() {
		saga.NewDefinition[order]("strict").
			Step("a", doStep(&recorder{}, "a", nil)).
			MustBuild(saga.WithCompensationPolicy(saga.PolicyEnforce))
	})

	// ReadOnly suppresses the enforcement.
	def, err := saga.NewDefinition[order]("ok").
		Step("a", doStep(&recorder{}, "a", nil)).ReadOnly().
		Build(saga.WithCompensationPolicy(saga.PolicyEnforce))
	require.NoError(t, err)
	require.Equal(t, "ok", def.Name())
}

func TestResumeUnknownInstance(t *testing.T) {
	t.Parallel()
	def := saga.NewDefinition[order]("x").Step("a", doStep(&recorder{}, "a", nil)).MustBuild()
	orch := saga.New(memory.New(), def)
	_, err := orch.Resume(t.Context(), "missing")
	require.ErrorIs(t, err, sagaerrs.ErrInstanceNotFound)
}

func TestResumeDefinitionMismatch(t *testing.T) {
	t.Parallel()
	store := memory.New()
	ctx := t.Context()
	require.NoError(t, store.Create(ctx, &saga.Instance{ID: "x", Definition: "other", Status: saga.StatusRunning}))

	def := saga.NewDefinition[order]("mine").Step("a", doStep(&recorder{}, "a", nil)).MustBuild()
	orch := saga.New(store, def)

	_, err := orch.Resume(ctx, "x")
	require.ErrorIs(t, err, sagaerrs.ErrDefinitionNotFound)
}

func TestPanicInCompensationFails(t *testing.T) {
	t.Parallel()
	r := &recorder{}
	var dead atomic.Pointer[saga.Instance]

	def := saga.NewDefinition[order]("comp-panic").
		Step("a", doStep(r, "a", nil)).Compensate(func(_ context.Context, _ *order) error {
		r.add("undo:a")
		panic("kaboom")
	}).
		Step("b", doStep(r, "b", errBoom)).Compensate(undoStep(r, "b")).
		MustBuild()

	orch := saga.New(memory.New(), def, fastOpts(
		saga.WithMaxStepAttempts(1),
		saga.WithMaxCompensationAttempts(2),
		saga.WithOnDeadLetter(func(inst *saga.Instance) { dead.Store(inst) }),
	)...)
	inst, err := orch.Start(t.Context(), "o1", order{})

	// b failed pre-pivot; compensating "a" panics and never recovers, so the
	// saga gives up in the Failed state and dead-letters.
	require.ErrorIs(t, err, sagaerrs.ErrCompensationFailed)
	require.Equal(t, saga.StatusFailed, inst.Status)
	require.Equal(t, 2, r.count("undo:a")) // retried up to MaxCompensationAttempts
	require.NotNil(t, dead.Load())
	require.Equal(t, saga.StatusFailed, dead.Load().Status)
}
