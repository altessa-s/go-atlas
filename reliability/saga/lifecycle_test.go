// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package saga_test

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/encoding/serializer"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/reliability/saga"
	"github.com/altessa-s/go-atlas/reliability/saga/storages/memory"

	schedtask "github.com/altessa-s/go-horai/task"
)

// hookStore wraps the memory store with failure and ordering hooks.
type hookStore struct {
	*memory.Store
	onUpdate  func(*saga.Instance) error
	onGet     func(id string) error
	onCreated func(id string) // Runs after the underlying Create committed, before it returns.
}

func (s *hookStore) Create(ctx context.Context, inst *saga.Instance) error {
	if err := s.Store.Create(ctx, inst); err != nil {
		return err
	}
	if s.onCreated != nil {
		s.onCreated(inst.ID)
	}
	return nil
}

func (s *hookStore) Update(ctx context.Context, inst *saga.Instance) error {
	if s.onUpdate != nil {
		if err := s.onUpdate(inst); err != nil {
			return err
		}
	}
	return s.Store.Update(ctx, inst)
}

func (s *hookStore) Get(ctx context.Context, id string) (*saga.Instance, error) {
	if s.onGet != nil {
		if err := s.onGet(id); err != nil {
			return nil, err
		}
	}
	return s.Store.Get(ctx, id)
}

// FetchRecoverable returns the matches ordered by ID so tests control which
// instance heads a limited batch.
func (s *hookStore) FetchRecoverable(ctx context.Context, definition string, now time.Time, limit int) ([]*saga.Instance, error) {
	all, err := s.Store.FetchRecoverable(ctx, definition, now, 0)
	if err != nil {
		return nil, err
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

func oneStepDef(name string, r *recorder) *saga.Definition[order] {
	return saga.NewDefinition[order](name).
		Step("a", doStep(r, "a", nil)).Compensate(undoStep(r, "a")).
		MustBuild()
}

func TestStartFirstUpdateFailureLeavesRecoverableInstance(t *testing.T) {
	t.Parallel()
	r := &recorder{}
	var failed atomic.Bool
	store := &hookStore{Store: memory.New(), onUpdate: func(*saga.Instance) error {
		if failed.CompareAndSwap(false, true) {
			return errBoom
		}
		return nil
	}}

	orch := saga.New(store, oneStepDef("lease", r), fastOpts()...)
	_, err := orch.Start(t.Context(), "x", order{})
	require.ErrorIs(t, err, errBoom)

	require.NoError(t, orch.RunRecoveryCycle(t.Context()))
	inst, err := store.Get(t.Context(), "x")
	require.NoError(t, err)
	require.Equal(t, saga.StatusCompleted, inst.Status)
	require.Equal(t, 1, r.count("do:a"))
}

func TestStartHoldsLeaseFromCreate(t *testing.T) {
	t.Parallel()
	r := &recorder{}
	var probeErr error
	var orch *saga.Orchestrator[order]
	store := &hookStore{Store: memory.New()}
	store.onCreated = func(id string) {
		// The instant after Create committed, before Start continues: another
		// executor must already find the lease.
		_, probeErr = orch.Resume(t.Context(), id)
	}
	orch = saga.New(store, oneStepDef("busy", r), fastOpts()...)

	_, err := orch.Start(t.Context(), "x", order{})
	require.NoError(t, err)
	require.ErrorIs(t, probeErr, saga.ErrInstanceBusy)
	require.Equal(t, 1, r.count("do:a"))
}

func TestStartCrashAfterCreateIsRecoveredAfterLeaseExpiry(t *testing.T) {
	t.Parallel()
	r := &recorder{}
	var crashed, restarted atomic.Bool
	crash := func() error {
		if crashed.Load() && !restarted.Load() {
			return errBoom
		}
		return nil
	}
	store := &hookStore{
		Store:    memory.New(),
		onUpdate: func(*saga.Instance) error { crashed.Store(true); return crash() },
		onGet:    func(string) error { return crash() },
	}
	opts := fastOpts(saga.WithExecutionTimeout(20*time.Millisecond), saga.WithLeaseGrace(20*time.Millisecond))
	orch := saga.New(store, oneStepDef("crash", r), opts...)

	_, err := orch.Start(t.Context(), "x", order{})
	require.ErrorIs(t, err, errBoom)
	require.Zero(t, r.count("do:a"))

	restarted.Store(true) // The "process" restarts.
	testhelpers.WaitFor(t, 5*time.Second, func() bool {
		_ = orch.RunRecoveryCycle(t.Context())
		inst, err := store.Get(t.Context(), "x")
		return err == nil && inst.Status == saga.StatusCompleted
	}, "the abandoned instance must be recovered once its lease expires")
	require.Equal(t, 1, r.count("do:a"))
}

func TestParallelCanceledSiblingIsCompensated(t *testing.T) {
	t.Parallel()
	var applied atomic.Bool
	var undone atomic.Bool
	appliedSet := make(chan struct{})
	def := saga.NewDefinition[order]("par").
		Parallel("p",
			saga.NewStep("a", func(ctx context.Context, _ *order) error {
				applied.Store(true)
				close(appliedSet)
				<-ctx.Done()
				return ctx.Err()
			}).Compensate(func(context.Context, *order) error {
				applied.Store(false)
				undone.Store(true)
				return nil
			}),
			saga.NewStep("b", func(context.Context, *order) error {
				<-appliedSet
				return errPermanent
			}).Compensate(func(context.Context, *order) error { return nil }),
		).
		MustBuild()

	opts := fastOpts(saga.WithMaxStepAttempts(1))
	orch := saga.New(memory.New(), def, opts...)
	inst, err := orch.Start(t.Context(), "x", order{})
	require.ErrorIs(t, err, errPermanent)
	require.Equal(t, saga.StatusCompensated, inst.Status)
	require.True(t, undone.Load(), "the canceled sibling's effect must be compensated")
	require.False(t, applied.Load())
}

// A first attempt that times out may have applied; a later attempt failing
// definitively must not erase that.
func TestTimedOutAttemptThenDefinitiveFailureIsCompensated(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	var undone atomic.Bool
	def := saga.NewDefinition[order]("retry-unknown").
		Step("a", func(ctx context.Context, _ *order) error {
			if calls.Add(1) == 1 {
				<-ctx.Done()
				return ctx.Err()
			}
			return errPermanent
		}).Compensate(func(context.Context, *order) error { undone.Store(true); return nil }).
		MustBuild()

	orch := saga.New(memory.New(), def, fastOpts(saga.WithMaxStepAttempts(2), saga.WithStepTimeout(20*time.Millisecond))...)
	inst, err := orch.Start(t.Context(), "x", order{})
	require.ErrorIs(t, err, errPermanent)
	require.Equal(t, saga.StatusCompensated, inst.Status)
	require.True(t, undone.Load())
}

// A step whose every action invocation failed definitively is not compensated
// because a sibling's failure canceled it during retry backoff.
func TestCancellationDuringBackoffAfterDefinitiveFailureIsNotUnknown(t *testing.T) {
	t.Parallel()
	var undone atomic.Bool
	failed := make(chan struct{})
	def := saga.NewDefinition[order]("backoff").
		Parallel("p",
			saga.NewStep("a", func(context.Context, *order) error {
				select {
				case <-failed:
				default:
					close(failed)
				}
				return errPermanent
			}).Compensate(func(context.Context, *order) error { undone.Store(true); return nil }).
				Retry(3, time.Hour, time.Hour),
			saga.NewStep("b", func(context.Context, *order) error {
				<-failed
				time.Sleep(20 * time.Millisecond) // a is now in its backoff
				return errBoom
			}),
		).
		MustBuild()

	orch := saga.New(memory.New(), def, fastOpts(saga.WithMaxStepAttempts(1))...)
	inst, err := orch.Start(t.Context(), "x", order{})
	require.Error(t, err)
	require.Equal(t, saga.StatusCompensated, inst.Status)
	require.False(t, undone.Load(), "a definitive failure is not an unknown outcome")
}

func TestParallelSiblingGatedBeforeActionIsNotCompensated(t *testing.T) {
	t.Parallel()
	var ran, undone atomic.Int32
	def := saga.NewDefinition[order]("par-gated").
		Parallel("p",
			saga.NewStep("a", func(context.Context, *order) error { return errPermanent }).
				Compensate(func(context.Context, *order) error { return nil }),
			saga.NewStep("b", func(context.Context, *order) error { ran.Add(1); return nil }).
				Compensate(func(context.Context, *order) error { undone.Add(1); return nil }),
		).
		MustBuild()

	orch := saga.New(memory.New(), def, fastOpts(saga.WithMaxStepAttempts(1), saga.WithStepConcurrency(1))...)
	inst, err := orch.Start(t.Context(), "x", order{})
	require.ErrorIs(t, err, errPermanent)
	require.Equal(t, saga.StatusCompensated, inst.Status)
	require.Zero(t, ran.Load())
	require.Zero(t, undone.Load())
}

func TestStepTimeoutOutcomeUnknownIsCompensated(t *testing.T) {
	t.Parallel()
	var undone atomic.Bool
	def := saga.NewDefinition[order]("timeout").
		Step("a", func(ctx context.Context, _ *order) error {
			<-ctx.Done()
			return ctx.Err()
		}).Compensate(func(context.Context, *order) error { undone.Store(true); return nil }).
		MustBuild()

	opts := fastOpts(saga.WithMaxStepAttempts(1), saga.WithStepTimeout(20*time.Millisecond))
	orch := saga.New(memory.New(), def, opts...)
	inst, err := orch.Start(t.Context(), "x", order{})
	require.Error(t, err)
	require.Equal(t, saga.StatusCompensated, inst.Status)
	require.True(t, undone.Load(), "a step that timed out may have applied and must be compensated")
}

// recordingRegistrar records scheduler registrations by task ID, last one wins.
type recordingRegistrar struct {
	mu    sync.Mutex
	tasks map[string]schedtask.Config
}

func (r *recordingRegistrar) Register(_ context.Context, cfg schedtask.Config) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.tasks == nil {
		r.tasks = map[string]schedtask.Config{}
	}
	r.tasks[cfg.ID] = cfg
	return nil
}

func TestDefaultRecoveryTaskIDIsPerDefinition(t *testing.T) {
	t.Parallel()
	reg := &recordingRegistrar{}
	rec := &recorder{}
	opts := fastOpts(saga.WithScheduler(reg), saga.WithRecoverySchedule("@every 1h"))

	for _, name := range []string{"first", "second"} {
		store := memory.New()
		craftNamed(t, store, name)
		orch := saga.New(store, oneStepDefNamed(name, rec), opts...)
		require.NoError(t, orch.RegisterRecovery(t.Context()))
	}

	require.Len(t, reg.tasks, 2)
	for _, cfg := range reg.tasks {
		require.NoError(t, cfg.Func(t.Context()))
	}
	require.Equal(t, 1, rec.count("undo:first"))
	require.Equal(t, 1, rec.count("undo:second"))
}

func oneStepDefNamed(name string, r *recorder) *saga.Definition[order] {
	return saga.NewDefinition[order](name).
		Step("a", doStep(r, name, nil)).Compensate(undoStep(r, name)).
		MustBuild()
}

// craftNamed stores a COMPENSATING instance of the named definition at stage 1.
func craftNamed(t *testing.T, store *memory.Store, name string) {
	t.Helper()
	require.NoError(t, store.Create(t.Context(), &saga.Instance{
		ID: "r-" + name, Definition: name, Status: saga.StatusCompensating, Stage: 1, Data: []byte("{}"),
	}))
}

func TestStartReleaseFailureAfterTerminalCheckpointIsNotAnError(t *testing.T) {
	t.Parallel()
	r := &recorder{}
	store := &hookStore{Store: memory.New(), onGet: func(string) error { return errBoom }}
	orch := saga.New(store, oneStepDef("release", r), fastOpts()...)

	inst, err := orch.Start(t.Context(), "x", order{})
	require.NoError(t, err)
	require.Equal(t, saga.StatusCompleted, inst.Status)

	stored, err := store.Store.Get(t.Context(), "x")
	require.NoError(t, err)
	require.Equal(t, saga.StatusCompleted, stored.Status)
}

// idempotentSink applies an effect once per idempotency key.
type idempotentSink struct {
	mu      sync.Mutex
	applied map[string]int
}

func (s *idempotentSink) apply(ctx context.Context) {
	e, _ := saga.ExecutionFromContext(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.applied == nil {
		s.applied = map[string]int{}
	}
	s.applied[e.StepKey]++
}

func (s *idempotentSink) effects() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.applied)
}

func TestBuildRejectsDuplicateStepNamesInStage(t *testing.T) {
	t.Parallel()
	sink := &idempotentSink{}
	act := func(ctx context.Context, _ *order) error { sink.apply(ctx); return nil }

	_, err := saga.NewDefinition[order]("dup").
		Parallel("p", saga.NewStep("charge", act), saga.NewStep("charge", act)).
		Build()
	require.ErrorIs(t, err, saga.ErrDuplicateStepName)

	_, err = saga.NewDefinition[order]("ok").
		Parallel("p", saga.NewStep("charge-1", act), saga.NewStep("charge-2", act)).
		Build()
	require.NoError(t, err)
}

// countingSerializer counts Serialize calls.
type countingSerializer struct {
	serializer.JSON
	n atomic.Int32
}

func (c *countingSerializer) Serialize(v any) ([]byte, error) {
	c.n.Add(1)
	return c.JSON.Serialize(v)
}

func TestSuccessfulStageSerializesPayloadOnce(t *testing.T) {
	t.Parallel()
	r := &recorder{}
	ser := &countingSerializer{}
	orch := saga.New(memory.New(), oneStepDef("ser", r), fastOpts(saga.WithSerializer(ser))...)

	_, err := orch.Start(t.Context(), "x", order{})
	require.NoError(t, err)
	require.EqualValues(t, 2, ser.n.Load(), "one for Create and one for the stage checkpoint")
}

func TestRecoveryQuarantinesPoisonInstance(t *testing.T) {
	t.Parallel()
	r := &recorder{}
	store := &hookStore{Store: memory.New()}
	require.NoError(t, store.Create(t.Context(), &saga.Instance{
		ID: "a-poison", Definition: "recover", Status: saga.StatusCompensating, Stage: 1, Data: []byte("{not json"),
	}))
	craftInstance(t, store.Store, saga.StatusCompensating, time.Time{}) // "r1" sorts after "a-poison".

	var dead []*saga.Instance
	opts := fastOpts(saga.WithRecoveryBatchSize(1), saga.WithOnDeadLetter(func(inst *saga.Instance) { dead = append(dead, inst) }))
	orch := saga.New(store, twoStepDef(r), opts...)

	for range 3 {
		_ = orch.RunRecoveryCycle(t.Context())
	}

	valid, err := store.Get(t.Context(), "r1")
	require.NoError(t, err)
	require.Equal(t, saga.StatusCompensated, valid.Status)
	poison, err := store.Get(t.Context(), "a-poison")
	require.NoError(t, err)
	require.Equal(t, saga.StatusFailed, poison.Status)
	require.NotEmpty(t, poison.LastError)
	require.Len(t, dead, 1)
	require.Equal(t, "a-poison", dead[0].ID)
}

func TestRecoveryQuarantineLosesToConcurrentWriter(t *testing.T) {
	t.Parallel()
	r := &recorder{}
	store := &hookStore{Store: memory.New()}
	require.NoError(t, store.Create(t.Context(), &saga.Instance{
		ID: "a-poison", Definition: "recover", Status: saga.StatusCompensating, Stage: 1, Data: []byte("{not json"),
	}))

	var once sync.Once
	var dead, repaired atomic.Int32
	store.onUpdate = func(inst *saga.Instance) error {
		once.Do(func() { // A healthy writer repairs the instance before the quarantine lands.
			repaired.Add(1)
			cur, err := store.Store.Get(t.Context(), inst.ID)
			require.NoError(t, err)
			cur.Data = []byte("{}")
			require.NoError(t, store.Store.Update(t.Context(), cur))
		})
		return nil
	}
	orch := saga.New(store, twoStepDef(r), fastOpts(saga.WithOnDeadLetter(func(*saga.Instance) { dead.Add(1) }))...)
	_ = orch.RunRecoveryCycle(t.Context())

	got, err := store.Store.Get(t.Context(), "a-poison")
	require.NoError(t, err)
	require.EqualValues(t, 1, repaired.Load(), "the quarantine write must have been attempted and lost")
	require.NotEqual(t, saga.StatusFailed, got.Status)
	require.Equal(t, []byte("{}"), got.Data, "the repaired payload must survive")
	require.Zero(t, dead.Load())
}

func TestRecoveryFailedLeaseAcquisitionAdvancesScanHead(t *testing.T) {
	t.Parallel()
	r := &recorder{}
	var calls atomic.Int32
	store := &hookStore{Store: memory.New()}
	store.onUpdate = func(*saga.Instance) error {
		if calls.Add(1) == 1 {
			return errBoom
		}
		return nil
	}
	craftInstance(t, store.Store, saga.StatusCompensating, time.Time{})

	orch := saga.New(store, twoStepDef(r), fastOpts()...)
	require.Error(t, orch.RunRecoveryCycle(t.Context()))

	rec, err := store.FetchRecoverable(t.Context(), "recover", time.Now(), 10)
	require.NoError(t, err)
	require.Empty(t, rec, "the instance whose lease could not be taken must leave the scan head")
}

func TestRetentionCycleDeletesOldTerminalInstances(t *testing.T) {
	t.Parallel()
	r := &recorder{}
	store := memory.New()
	orch := saga.New(store, oneStepDef("retain", r), fastOpts(saga.WithRetention(time.Hour), saga.WithRetentionBatchSize(2))...)

	for _, id := range []string{"a", "b", "c", "d", "e"} {
		_, err := orch.Start(t.Context(), id, order{})
		require.NoError(t, err)
	}
	require.Equal(t, 5, store.Len())

	// Nothing is old enough yet.
	require.NoError(t, orch.RunRetentionCycle(t.Context()))
	require.Equal(t, 5, store.Len())

	// Age the instances past the retention window, plus one live instance.
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		inst, err := store.Get(t.Context(), id)
		require.NoError(t, err)
		inst.UpdatedAt = time.Now().Add(-2 * time.Hour)
		require.NoError(t, store.Update(t.Context(), inst))
	}
	require.NoError(t, store.Create(t.Context(), &saga.Instance{
		ID: "live", Definition: "retain", Status: saga.StatusCompensating, UpdatedAt: time.Now().Add(-2 * time.Hour),
	}))

	require.NoError(t, orch.RunRetentionCycle(t.Context()))
	require.Equal(t, 1, store.Len(), "all batches are drained in one cycle and the live instance stays")
}

func TestRetentionDisabledExplicitly(t *testing.T) {
	t.Parallel()
	r := &recorder{}
	store := memory.New()
	orch := saga.New(store, oneStepDef("retain", r), fastOpts(saga.WithRetention(0))...)
	_, err := orch.Start(t.Context(), "a", order{})
	require.NoError(t, err)
	inst, err := store.Get(t.Context(), "a")
	require.NoError(t, err)
	inst.UpdatedAt = time.Now().Add(-1000 * time.Hour)
	require.NoError(t, store.Update(t.Context(), inst))

	require.NoError(t, orch.RunRetentionCycle(t.Context()))
	require.Equal(t, 1, store.Len())
}

func TestRetentionOnByDefault(t *testing.T) {
	t.Parallel()
	r := &recorder{}
	store := memory.New()
	orch := saga.New(store, oneStepDef("retain", r), fastOpts()...)
	_, err := orch.Start(t.Context(), "old", order{})
	require.NoError(t, err)
	_, err = orch.Start(t.Context(), "fresh", order{})
	require.NoError(t, err)
	inst, err := store.Get(t.Context(), "old")
	require.NoError(t, err)
	inst.UpdatedAt = time.Now().Add(-31 * 24 * time.Hour)
	require.NoError(t, store.Update(t.Context(), inst))

	require.NoError(t, orch.RunRetentionCycle(t.Context()))
	require.Equal(t, 1, store.Len(), "a terminal instance older than 30 days is deleted without any retention option")
}

func TestRetentionTaskIsPerDefinitionAndManaged(t *testing.T) {
	t.Parallel()
	reg := &recordingRegistrar{}
	rec := &recorder{}
	opts := fastOpts(saga.WithScheduler(reg), saga.WithRetention(time.Hour))

	for _, name := range []string{"first", "second"} {
		orch := saga.New(memory.New(), oneStepDefNamed(name, rec), opts...)
		require.NoError(t, orch.RegisterRetention(t.Context()))
		require.ErrorIs(t, orch.RunRetentionCycle(t.Context()), saga.ErrSchedulerManaged)
	}
	require.Contains(t, reg.tasks, saga.RetentionTaskIDPrefix+"first")
	require.Contains(t, reg.tasks, saga.RetentionTaskIDPrefix+"second")

	none := &recordingRegistrar{}
	off := saga.New(memory.New(), oneStepDef("off", rec), fastOpts(saga.WithScheduler(none), saga.WithRetention(0))...)
	require.NoError(t, off.RegisterRetention(t.Context()))
	require.Empty(t, none.tasks, "retention off registers nothing")
}

func TestRecoveryLeavesLeasedPoisonInstanceAlone(t *testing.T) {
	t.Parallel()
	r := &recorder{}
	store := &hookStore{Store: memory.New()}
	require.NoError(t, store.Create(t.Context(), &saga.Instance{
		ID: "a-poison", Definition: "recover", Status: saga.StatusCompensating, Stage: 1, Data: []byte("{not json"),
	}))
	// The scan saw an expired lease; another node takes it before Get.
	store.onGet = func(id string) error {
		cur, err := store.Store.Get(t.Context(), id)
		require.NoError(t, err)
		cur.LeaseOwner, cur.LeaseUntil = "other-node", time.Now().Add(time.Minute)
		require.NoError(t, store.Store.Update(t.Context(), cur))
		store.onGet = nil
		return nil
	}
	var dead atomic.Int32
	orch := saga.New(store, twoStepDef(r), fastOpts(saga.WithOnDeadLetter(func(*saga.Instance) { dead.Add(1) }))...)
	require.NoError(t, orch.RunRecoveryCycle(t.Context()))

	got, err := store.Store.Get(t.Context(), "a-poison")
	require.NoError(t, err)
	require.Equal(t, saga.StatusCompensating, got.Status)
	require.Equal(t, "other-node", got.LeaseOwner)
	require.Zero(t, dead.Load())
}

func TestRecoveryBackoffDoesNotOverwriteAnotherExecutorsLease(t *testing.T) {
	t.Parallel()
	r := &recorder{}
	store := &hookStore{Store: memory.New()}
	craftInstance(t, store.Store, saga.StatusCompensating, time.Time{})
	var once sync.Once
	store.onUpdate = func(inst *saga.Instance) error {
		var err error
		once.Do(func() { // The lease write fails, and another executor wins the instance meanwhile.
			cur, gerr := store.Store.Get(t.Context(), inst.ID)
			require.NoError(t, gerr)
			cur.LeaseOwner, cur.LeaseUntil = "other-node", time.Now().Add(time.Hour)
			require.NoError(t, store.Store.Update(t.Context(), cur))
			err = errBoom
		})
		return err
	}
	orch := saga.New(store, twoStepDef(r), fastOpts()...)
	require.Error(t, orch.RunRecoveryCycle(t.Context()))

	got, err := store.Store.Get(t.Context(), "r1")
	require.NoError(t, err)
	require.Equal(t, "other-node", got.LeaseOwner)
	require.WithinDuration(t, time.Now().Add(time.Hour), got.LeaseUntil, time.Minute, "the other executor's lease must stay")
}

// corruptStore reports one stub for a record it cannot decode.
type corruptStore struct {
	*hookStore
}

func (s corruptStore) FetchRecoverable(ctx context.Context, definition string, now time.Time, limit int) ([]*saga.Instance, error) {
	rec, err := s.hookStore.FetchRecoverable(ctx, definition, now, limit)
	if cur, gerr := s.Store.Get(ctx, "broken"); gerr == nil && cur.Status == saga.StatusRunning {
		rec = append([]*saga.Instance{{ID: "broken", Version: cur.Version, Corrupt: true}}, rec...)
	}
	return rec, err
}

func TestRecoveryQuarantinesCorruptStorageRecord(t *testing.T) {
	t.Parallel()
	r := &recorder{}
	hs := &hookStore{Store: memory.New()}
	require.NoError(t, hs.Create(t.Context(), &saga.Instance{ID: "broken", Definition: "x", Status: saga.StatusRunning}))
	var dead []*saga.Instance
	orch := saga.New(corruptStore{hs}, twoStepDef(r), fastOpts(saga.WithOnDeadLetter(func(i *saga.Instance) { dead = append(dead, i) }))...)

	require.NoError(t, orch.RunRecoveryCycle(t.Context()))
	got, err := hs.Store.Get(t.Context(), "broken")
	require.NoError(t, err)
	require.Equal(t, saga.StatusFailed, got.Status)
	require.Len(t, dead, 1)

	require.NoError(t, orch.RunRecoveryCycle(t.Context()))
	require.Len(t, dead, 1, "a quarantined record is not reported again")
}

func TestRetentionRejectsEmptyDefinitionName(t *testing.T) {
	t.Parallel()
	rec := &recorder{}
	store := memory.New()
	require.NoError(t, store.Create(t.Context(), &saga.Instance{ID: "other", Definition: "other-saga", Status: saga.StatusCompleted}))
	orch := saga.New(store, oneStepDefNamed("", rec), fastOpts(saga.WithRetention(time.Nanosecond))...)

	require.ErrorIs(t, orch.RunRetentionCycle(t.Context()), saga.ErrEmptyDefinitionName)
	require.Equal(t, 1, store.Len(), "another definition's history must survive")
	require.ErrorIs(t, orch.RegisterTasks(t.Context()), saga.ErrEmptyDefinitionName)
}

// The 30-day default applies to named definitions only: an unnamed one gets
// retention off, so recovery registration keeps working with default options.
func TestUnnamedDefinitionHasNoDefaultRetention(t *testing.T) {
	t.Parallel()
	reg := &recordingRegistrar{}
	orch := saga.New(memory.New(), oneStepDefNamed("", &recorder{}), fastOpts(saga.WithScheduler(reg))...)

	require.NoError(t, orch.RegisterRecovery(t.Context()))
	require.NoError(t, orch.RegisterRetention(t.Context()))
	require.NoError(t, orch.RunRetentionCycle(t.Context()))
	require.Len(t, reg.tasks, 1, "only the recovery task is registered")

	explicit := saga.New(memory.New(), oneStepDefNamed("", &recorder{}), fastOpts(saga.WithScheduler(reg), saga.WithRetention(saga.DefaultRetention))...)
	require.ErrorIs(t, explicit.RegisterRetention(t.Context()), saga.ErrEmptyDefinitionName, "an explicit retention needs a name")
}

func TestRetentionBatchSizeNonPositiveFallsBackToDefault(t *testing.T) {
	t.Parallel()
	rec := &recorder{}
	store := memory.New()
	orch := saga.New(store, oneStepDef("retain", rec), fastOpts(saga.WithRetention(time.Nanosecond), saga.WithRetentionBatchSize(0))...)
	_, err := orch.Start(t.Context(), "a", order{})
	require.NoError(t, err)
	time.Sleep(2 * time.Millisecond)

	done := make(chan error, 1)
	go func() { done <- orch.RunRetentionCycle(t.Context()) }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("retention cycle spun on a non-positive batch size")
	}
	require.Zero(t, store.Len())
}

func TestTaskIDCollisionIsRejected(t *testing.T) {
	t.Parallel()
	reg := &recordingRegistrar{}
	rec := &recorder{}
	orch := saga.New(memory.New(), oneStepDef("same", rec), fastOpts(
		saga.WithScheduler(reg), saga.WithRetention(time.Hour), saga.WithRecoveryTaskID(saga.RetentionTaskIDPrefix+"same"))...)

	require.ErrorIs(t, orch.RegisterRecovery(t.Context()), saga.ErrTaskIDCollision)
	require.ErrorIs(t, orch.RegisterRetention(t.Context()), saga.ErrTaskIDCollision)
	require.ErrorIs(t, orch.RegisterTasks(t.Context()), saga.ErrTaskIDCollision)
	require.Empty(t, reg.tasks)
}

// failingRegistrar fails the registration of one task ID.
type failingRegistrar struct {
	recordingRegistrar
	failID string
}

func (r *failingRegistrar) Register(ctx context.Context, cfg schedtask.Config) error {
	if cfg.ID == r.failID {
		return errBoom
	}
	return r.recordingRegistrar.Register(ctx, cfg)
}

func TestRegisterTasksIsAllOrNothing(t *testing.T) {
	t.Parallel()
	rec := &recorder{}
	reg := &failingRegistrar{failID: saga.RetentionTaskIDPrefix + "aon"}
	store := memory.New()
	craftNamed(t, store, "aon")
	orch := saga.New(store, oneStepDefNamed("aon", rec), fastOpts(saga.WithScheduler(reg), saga.WithRetention(time.Hour))...)

	require.ErrorIs(t, orch.RegisterTasks(t.Context()), errBoom)
	task, ok := reg.tasks[saga.RecoveryTaskIDPrefix+"aon"]
	require.True(t, ok, "the first registration happened")
	require.NoError(t, task.Func(t.Context()))
	require.Zero(t, rec.count("undo:aon"), "a failed RegisterTasks must leave no active callback")
}
