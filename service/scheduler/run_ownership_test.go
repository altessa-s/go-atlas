// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler_test

import (
	"context"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/service/scheduler"
	"github.com/altessa-s/go-atlas/service/scheduler/storages/memory"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// createdAndClaimedElsewhere reports the task as absent on the first read and,
// in that same window, lets another instance create the task and claim its
// first occurrence — the interleaving of two instances registering one new
// task at once.
type createdAndClaimedElsewhere struct {
	*memory.Storage
	once sync.Once
}

func (s *createdAndClaimedElsewhere) GetTask(ctx context.Context, id string) (*scheduler.TaskState, error) {
	var (
		absent bool
		err    error
	)
	s.once.Do(func() {
		absent = true
		if err = s.Storage.UpsertTask(ctx, &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{
			ID: id, Status: scheduler.TaskStatusActive, Schedule: "@every 1h", NextRunAt: 100,
		}}); err != nil {
			return
		}
		_, err = s.Storage.ClaimRun(ctx, id, scheduler.RunClaim{NextRunAt: 100, StartedAt: time.Now().Unix(), RunID: "other-instance/run"})
	})
	if err != nil {
		return nil, err
	}
	if absent {
		return nil, nil //nolint:nilnil // reproduces the "not found" read that lost the race
	}
	return s.Storage.GetTask(ctx, id)
}

func TestRegisterDoesNotOverwriteTaskCreatedConcurrently(t *testing.T) {
	t.Parallel()
	mem := mustNewMemory(t, 10)
	s := scheduler.New(&createdAndClaimedElsewhere{Storage: mem})

	require.NoError(t, s.Register(t.Context(), corescheduler.TaskConfig{
		ID: "task", Schedule: "@every 1h", RunOnStart: true, Func: func(context.Context) error { return nil },
	}))

	got, err := mem.GetTask(t.Context(), "task")
	require.NoError(t, err)
	require.Equal(t, scheduler.TaskStatusRunning, got.Status, "the competing claim must survive registration")
	require.Equal(t, "other-instance/run", got.LastRunID)
	require.NotZero(t, got.RunStartedAt)
	require.Equal(t, int64(100), got.NextRunAt, "the claimed occurrence must not be replaced by a fresh one")
}

// seedRun stores a task whose run runID is unfinished. A non-zero leaseUntil
// is bound to runID, as ClaimRun and RenewRun store it.
func seedRun(t *testing.T, mem *memory.Storage, status scheduler.TaskStatus, runID string, startedAt, leaseUntil int64) {
	t.Helper()
	var leaseID string
	if leaseUntil != 0 {
		leaseID = runID
	}
	require.NoError(t, mem.UpsertTask(t.Context(), &scheduler.TaskState{
		TaskSummary:   scheduler.TaskSummary{ID: "task", Status: status, Schedule: "@every 1h", NextRunAt: startedAt},
		LastRunID:     runID,
		RunStartedAt:  startedAt,
		RunLeaseUntil: leaseUntil,
		RunLeaseID:    leaseID,
		UpdatedAt:     startedAt,
	}))
}

func startForRecovery(t *testing.T, store scheduler.Storage, opts ...scheduler.Option) *scheduler.Scheduler {
	t.Helper()
	s := scheduler.New(store, append([]scheduler.Option{scheduler.WithTickInterval(time.Hour)}, opts...)...)
	require.NoError(t, s.Start(t.Context()))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 10*time.Second)
		defer cancel()
		require.NoError(t, s.Stop(ctx))
	})
	return s
}

func requireStillRunning(t *testing.T, mem *memory.Storage, runID string) {
	t.Helper()
	got, err := mem.GetTask(t.Context(), "task")
	require.NoError(t, err)
	require.Equal(t, scheduler.TaskStatusRunning, got.Status, "a live run must not be recovered")
	require.Equal(t, runID, got.LastRunID)
	require.NotZero(t, got.RunStartedAt)
	require.Zero(t, got.Failures)
}

func requireRecovered(t *testing.T, mem *memory.Storage, want scheduler.TaskStatus) {
	t.Helper()
	got, err := mem.GetTask(t.Context(), "task")
	require.NoError(t, err)
	require.Equal(t, want, got.Status)
	require.Zero(t, got.RunStartedAt)
	require.Equal(t, int32(1), got.Failures)
}

// neverRecovered asserts that stale recovery leaves the run alone for d.
func neverRecovered(t *testing.T, mem *memory.Storage, d time.Duration) {
	t.Helper()
	require.Never(t, func() bool {
		got, err := mem.GetTask(t.Context(), "task")
		return err != nil || got.RunStartedAt == 0 || got.Failures != 0
	}, d, 50*time.Millisecond, "a run with a live lease was recovered")
}

func TestStartupRecoveryLeavesLiveRunOfAnotherInstance(t *testing.T) {
	t.Parallel()
	mem := mustNewMemory(t, 10)
	seedRun(t, mem, scheduler.TaskStatusRunning, "node-b/run", time.Now().Unix(), 0)

	startForRecovery(t, mem, scheduler.WithStaleTaskTimeout(30*time.Minute))

	requireStillRunning(t, mem, "node-b/run")
}

func TestStartupRecoveryReclaimsExpiredLease(t *testing.T) {
	t.Parallel()
	mem := mustNewMemory(t, 10)
	old := time.Now().Add(-time.Hour).Unix()
	seedRun(t, mem, scheduler.TaskStatusRunning, "node-b/run", old, old)

	startForRecovery(t, mem, scheduler.WithStaleTaskTimeout(30*time.Minute))

	requireRecovered(t, mem, scheduler.TaskStatusActive)
}

func TestStartupRecoveryReclaimsOwnRun(t *testing.T) {
	t.Parallel()
	mem := mustNewMemory(t, 10)
	now := time.Now().Unix()
	seedRun(t, mem, scheduler.TaskStatusRunning, "node-a/run", now, now+3600)

	startForRecovery(t, mem, scheduler.WithStaleTaskTimeout(30*time.Minute), scheduler.WithInstanceID("node-a"))

	requireRecovered(t, mem, scheduler.TaskStatusActive)
}

func TestStaleRecoveryHonorsPersistedLease(t *testing.T) {
	t.Parallel()
	mem := mustNewMemory(t, 10)
	seedRun(t, mem, scheduler.TaskStatusRunning, "node-b/run", time.Now().Add(-time.Hour).Unix(), time.Now().Add(time.Hour).Unix())

	// The recovery interval equals the one-second stale timeout, so this spans
	// several periodic passes.
	startForRecovery(t, mem, scheduler.WithStaleTaskTimeout(time.Second))
	neverRecovered(t, mem, 2500*time.Millisecond)
}

func TestStaleRecoveryReclaimsOwnAbandonedRunWithoutWaiting(t *testing.T) {
	t.Parallel()
	mem := mustNewMemory(t, 10)
	startForRecovery(t, mem, scheduler.WithStaleTaskTimeout(time.Second), scheduler.WithInstanceID("node-a"))

	// A run of this instance that is no longer executing, e.g. one whose
	// FinishRun write failed, is recovered on the next pass despite its lease.
	seedRun(t, mem, scheduler.TaskStatusRunning, "node-a/run", time.Now().Unix(), time.Now().Add(time.Hour).Unix())

	require.Eventually(t, func() bool {
		got, err := mem.GetTask(t.Context(), "task")
		return err == nil && got.RunStartedAt == 0
	}, 3*time.Second, 50*time.Millisecond)
	requireRecovered(t, mem, scheduler.TaskStatusActive)
}

func TestRecoveryPreservesManagementStatus(t *testing.T) {
	t.Parallel()
	mem := mustNewMemory(t, 10)
	old := time.Now().Add(-time.Hour).Unix()
	seedRun(t, mem, scheduler.TaskStatusPaused, "node-b/run", old, old)

	startForRecovery(t, mem)

	requireRecovered(t, mem, scheduler.TaskStatusPaused)
	got, err := mem.GetTask(t.Context(), "task")
	require.NoError(t, err)
	require.Equal(t, old, got.NextRunAt, "a paused task keeps its next run time")
}

func TestRegisterPreservesRunLease(t *testing.T) {
	t.Parallel()
	mem := mustNewMemory(t, 10)
	leaseUntil := time.Now().Add(time.Hour).Unix()
	seedRun(t, mem, scheduler.TaskStatusRunning, "node-b/run", time.Now().Add(-time.Hour).Unix(), leaseUntil)

	require.NoError(t, scheduler.New(mem).Register(t.Context(), corescheduler.TaskConfig{
		ID: "task", Schedule: "@every 1h", Func: func(context.Context) error { return nil },
	}))
	got, err := mem.GetTask(t.Context(), "task")
	require.NoError(t, err)
	require.Equal(t, leaseUntil, got.RunLeaseUntil)

	startForRecovery(t, mem)
	requireStillRunning(t, mem, "node-b/run")
}

func TestLiveRunRenewsLeaseAndIsNotRecovered(t *testing.T) {
	t.Parallel()
	mem := mustNewMemory(t, 10)
	release := make(chan struct{})
	s := scheduler.New(mem, scheduler.WithTickInterval(20*time.Millisecond), scheduler.WithStaleTaskTimeout(time.Second))
	require.NoError(t, s.Start(t.Context()))
	t.Cleanup(func() {
		close(release)
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 5*time.Second)
		defer cancel()
		require.NoError(t, s.Stop(ctx))
	})
	require.NoError(t, s.Register(t.Context(), corescheduler.TaskConfig{
		ID: "task", Schedule: "@every 1h", RunOnStart: true,
		Func: func(context.Context) error { <-release; return nil },
	}))

	var first *scheduler.TaskState
	require.Eventually(t, func() bool {
		got, err := mem.GetTask(t.Context(), "task")
		first = got
		return err == nil && got.RunLeaseUntil > 0
	}, 2*time.Second, 10*time.Millisecond, "the lease must be persisted right after the claim")
	require.GreaterOrEqual(t, first.RunLeaseUntil, first.RunStartedAt+5, "the lease is floored at five seconds")

	require.Eventually(t, func() bool {
		got, err := mem.GetTask(t.Context(), "task")
		return err == nil && got.RunLeaseUntil > first.RunLeaseUntil
	}, 4*time.Second, 50*time.Millisecond, "a live run must keep renewing its lease")

	requireStillRunning(t, mem, first.LastRunID)
}

// slowHistory blocks AddHistory for delay, standing in for a slow backend
// while the finished run's result is being recorded.
type slowHistory struct {
	*memory.Storage
	delay time.Duration
}

func (s *slowHistory) AddHistory(ctx context.Context, h *scheduler.TaskHistory) error {
	select {
	case <-time.After(s.delay):
	case <-ctx.Done():
		return ctx.Err()
	}
	return s.Storage.AddHistory(ctx, h)
}

// TestSecondInstanceLeavesLiveRun runs a task on one instance past the minimum
// lease while a second instance with a one-second stale timeout keeps running
// recovery. The second instance has its own ID, so only the persisted lease
// protects the run.
func TestSecondInstanceLeavesLiveRun(t *testing.T) {
	t.Parallel()

	const busy = 7 * time.Second
	for _, tc := range []struct {
		name      string
		ownerOpts []scheduler.Option
		slowStore bool
	}{
		// Renewal across second boundaries at the floor lease.
		{name: "minimum_lease", ownerOpts: []scheduler.Option{scheduler.WithStaleTaskTimeout(time.Second)}},
		// The owner's (default, 30m) lease is what counts, not the observer's.
		{name: "owner_defined_lease"},
		// The lease stays alive while the result is being recorded.
		{name: "slow_finalization", ownerOpts: []scheduler.Option{scheduler.WithStaleTaskTimeout(time.Second)}, slowStore: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mem := mustNewMemory(t, 10)
			var ownerStore scheduler.Storage = mem
			body := busy
			if tc.slowStore {
				ownerStore = &slowHistory{Storage: mem, delay: busy}
				body = 0
			}

			var runs atomic.Int32
			owner := startForRecovery(t, ownerStore, append([]scheduler.Option{scheduler.WithTickInterval(20 * time.Millisecond)}, tc.ownerOpts...)...)
			require.NoError(t, owner.Register(t.Context(), corescheduler.TaskConfig{
				ID: "task", Schedule: "@every 1h", RunOnStart: true,
				Func: func(ctx context.Context) error {
					runs.Add(1)
					select {
					case <-time.After(body):
					case <-ctx.Done():
					}
					return nil
				},
			}))
			require.Eventually(t, func() bool { return runs.Load() == 1 }, 2*time.Second, 10*time.Millisecond)

			startForRecovery(t, mem, scheduler.WithStaleTaskTimeout(time.Second))
			neverRecovered(t, mem, busy-500*time.Millisecond)

			require.Eventually(t, func() bool {
				got, err := mem.GetTask(t.Context(), "task")
				return err == nil && got.RunStartedAt == 0 && got.LastRunAt > 0
			}, 5*time.Second, 50*time.Millisecond, "the owner must record its result")
			got, err := mem.GetTask(t.Context(), "task")
			require.NoError(t, err)
			require.Zero(t, got.Failures)
			require.Equal(t, int32(1), runs.Load())
		})
	}
}

// TestManagementDuringRunDoesNotAllowSecondClaim pauses (disables) and resumes
// (enables) a task while its run is in flight on one instance, making the next
// occurrence due, and checks that a second instance cannot claim it until the
// first run has finished.
func TestManagementDuringRunDoesNotAllowSecondClaim(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name          string
		stop, restart func(*scheduler.Scheduler, context.Context, string) error
	}{
		{"pause_resume", (*scheduler.Scheduler).PauseTask, (*scheduler.Scheduler).ResumeTask},
		{"disable_enable", (*scheduler.Scheduler).DisableTask, (*scheduler.Scheduler).EnableTask},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mem := mustNewMemory(t, 10)
			release := make(chan struct{})
			var runs atomic.Int32
			cfg := corescheduler.TaskConfig{
				ID: "task", Schedule: "@every 1s", RunOnStart: true,
				Func: func(ctx context.Context) error {
					runs.Add(1)
					select {
					case <-release:
					case <-ctx.Done():
					}
					return nil
				},
			}

			first := startForRecovery(t, mem, scheduler.WithTickInterval(20*time.Millisecond))
			require.NoError(t, first.Register(t.Context(), cfg))
			require.Eventually(t, func() bool { return runs.Load() == 1 }, 2*time.Second, 10*time.Millisecond)

			second := startForRecovery(t, mem, scheduler.WithTickInterval(20*time.Millisecond))
			require.NoError(t, second.Register(t.Context(), cfg))

			require.NoError(t, tc.stop(first, t.Context(), "task"))
			require.NoError(t, tc.restart(first, t.Context(), "task"))

			require.Never(t, func() bool { return runs.Load() > 1 }, 2500*time.Millisecond, 20*time.Millisecond,
				"a second instance claimed the task while its run was unfinished")

			close(release)
			require.Eventually(t, func() bool { return runs.Load() > 1 }, 5*time.Second, 20*time.Millisecond,
				"the task must be claimable again once the run has finished")
		})
	}
}

// TestShortOwnerLeaseIsAuthoritative recovers a run whose owner persisted a
// short lease that has expired, on an observer with the default 30m timeout:
// the owner's lease decides, not the observer's timeout.
func TestShortOwnerLeaseIsAuthoritative(t *testing.T) {
	t.Parallel()
	mem := mustNewMemory(t, 10)
	now := time.Now().Unix()
	seedRun(t, mem, scheduler.TaskStatusRunning, "node-b/run", now-10, now-2)

	startForRecovery(t, mem)

	requireRecovered(t, mem, scheduler.TaskStatusActive)
}

// TestRecoveryKeepsReRegisteredOccurrence abandons a one-shot run whose task was
// re-registered for a later RunAt meanwhile: recovery must keep that later time
// instead of running the new occurrence at once.
func TestRecoveryKeepsReRegisteredOccurrence(t *testing.T) {
	t.Parallel()
	mem := mustNewMemory(t, 10)
	now := time.Now().Unix()
	later := now + 3600
	require.NoError(t, mem.UpsertTask(t.Context(), &scheduler.TaskState{
		TaskSummary:   scheduler.TaskSummary{ID: "task", Status: scheduler.TaskStatusRunning, OneShot: true, NextRunAt: later},
		RunAt:         later,
		LastRunID:     "node-b/run",
		RunStartedAt:  now - 60,
		RunLeaseUntil: now - 30,
		RunLeaseID:    "node-b/run",
	}))

	startForRecovery(t, mem)

	requireRecovered(t, mem, scheduler.TaskStatusActive)
	got, err := mem.GetTask(t.Context(), "task")
	require.NoError(t, err)
	require.Equal(t, later, got.NextRunAt)
}

// reRegisterBeforeClaim moves the task to another registered occurrence with
// the same NextRunAt right before the first claim, as two registrations with
// past RunAt values clamped into the same second do.
type reRegisterBeforeClaim struct {
	*memory.Storage
	once sync.Once
}

func (s *reRegisterBeforeClaim) ClaimRun(ctx context.Context, id string, claim scheduler.RunClaim) (bool, error) {
	var err error
	s.once.Do(func() {
		var st *scheduler.TaskState
		if st, err = s.Storage.GetTask(ctx, id); err != nil {
			return
		}
		st.RunAt--
		err = s.Storage.UpsertTask(ctx, st)
	})
	if err != nil {
		return false, err
	}
	return s.Storage.ClaimRun(ctx, id, claim)
}

func TestClaimFencesRegisteredOccurrence(t *testing.T) {
	t.Parallel()
	mem := mustNewMemory(t, 10)
	store := &reRegisterBeforeClaim{Storage: mem}
	s := startForRecovery(t, store, scheduler.WithTickInterval(20*time.Millisecond))
	var runs atomic.Int32
	require.NoError(t, s.Register(t.Context(), corescheduler.TaskConfig{
		ID: "task", RunAt: time.Now().Add(-time.Minute),
		Func: func(context.Context) error { runs.Add(1); return nil },
	}))

	// The claim observed the old occurrence and must fail; the task now holds
	// an occurrence registered elsewhere, which this registration must not run.
	require.Never(t, func() bool { return runs.Load() > 0 }, time.Second, 20*time.Millisecond,
		"a run was started for an occurrence the claim did not observe")
	got, err := mem.GetTask(t.Context(), "task")
	require.NoError(t, err)
	require.Equal(t, scheduler.TaskStatusActive, got.Status)
	require.Zero(t, got.RunStartedAt)
}

// leaderSwitch is a LeaderElector the test flips.
type leaderSwitch struct{ on atomic.Bool }

func (l *leaderSwitch) IsLeader() bool { return l.on.Load() }

// TestStaleRegistrationDoesNotRunNewOccurrence registers one one-shot task on
// two instances with different RunAt values and functions. The task's stored
// occurrence is the later registration's, so only that registration's function
// may run it — never the function the first instance still holds.
func TestStaleRegistrationDoesNotRunNewOccurrence(t *testing.T) {
	t.Parallel()
	mem := mustNewMemory(t, 10)
	var oldRuns, newRuns atomic.Int32
	oldLeader, newLeader := &leaderSwitch{}, &leaderSwitch{}
	old := startForRecovery(t, mem, scheduler.WithTickInterval(20*time.Millisecond), scheduler.WithLeaderElector(oldLeader))
	current := startForRecovery(t, mem, scheduler.WithTickInterval(20*time.Millisecond), scheduler.WithLeaderElector(newLeader))
	runAt := time.Now().Add(-time.Minute)

	require.NoError(t, old.Register(t.Context(), corescheduler.TaskConfig{
		ID: "task", RunAt: runAt, Func: func(context.Context) error { oldRuns.Add(1); return nil },
	}))
	require.NoError(t, current.Register(t.Context(), corescheduler.TaskConfig{
		ID: "task", RunAt: runAt.Add(time.Second), Func: func(context.Context) error { newRuns.Add(1); return nil },
	}))

	oldLeader.on.Store(true)
	require.Never(t, func() bool { return oldRuns.Load() > 0 }, time.Second, 20*time.Millisecond,
		"a stale registration ran the newly registered occurrence")

	oldLeader.on.Store(false)
	newLeader.on.Store(true)
	require.Eventually(t, func() bool { return newRuns.Load() == 1 }, 3*time.Second, 10*time.Millisecond)
	require.Zero(t, oldRuns.Load())
}

// slowClaim delays the ClaimRun response past a third of the lease and can let
// another instance take the run over before the response arrives — or, with
// slowRenewal, before the response to the first (successful) renewal arrives.
type slowClaim struct {
	*memory.Storage
	delay       time.Duration
	takeOver    bool
	slowRenewal bool
	renewals    atomic.Int32
	// claimed is set once the delayed ClaimRun response has been returned.
	claimed atomic.Bool
}

func (s *slowClaim) takeOverRun(ctx context.Context, id string) error {
	// Recovery on another instance reset the run and a new claim won it.
	st, err := s.Storage.GetTask(ctx, id)
	if err != nil {
		return err
	}
	st.LastRunID = "other/run"
	return s.Storage.UpsertTask(ctx, st)
}

func (s *slowClaim) RenewRun(ctx context.Context, id, runID string, leaseUntil int64) (bool, error) {
	ok, err := s.Storage.RenewRun(ctx, id, runID, leaseUntil)
	if err != nil || !ok || !s.slowRenewal || s.renewals.Add(1) > 1 {
		return ok, err
	}
	if err = s.takeOverRun(ctx, id); err != nil {
		return false, err
	}
	time.Sleep(s.delay)
	return true, nil
}

func (s *slowClaim) ClaimRun(ctx context.Context, id string, claim scheduler.RunClaim) (bool, error) {
	ok, err := s.Storage.ClaimRun(ctx, id, claim)
	if err != nil || !ok {
		return ok, err
	}
	if s.takeOver {
		if err = s.takeOverRun(ctx, id); err != nil {
			return false, err
		}
	}
	time.Sleep(s.delay)
	s.claimed.Store(true)
	return true, nil
}

// TestSlowClaimConfirmsOwnershipBeforeRunning delays the claim response by more
// than a third of the (5s minimum) lease: the run renews its lease before the
// body starts, and does not start at all when the run was taken over meanwhile.
func TestSlowClaimConfirmsOwnershipBeforeRunning(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		takeOver    bool
		slowRenewal bool
		wantRuns    int32
	}{
		{"still_owned", false, false, 1},
		{"taken_over", true, false, 0},
		// The confirming renewal succeeds, but its response arrives after the
		// run was taken over: a late success must not start the body.
		{"late_renewal_response", false, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mem := mustNewMemory(t, 10)
			store := &slowClaim{Storage: mem, delay: 2 * time.Second, takeOver: tc.takeOver, slowRenewal: tc.slowRenewal}
			// A one-second stale timeout gives the five-second minimum lease.
			s := startForRecovery(t, store, scheduler.WithTickInterval(20*time.Millisecond), scheduler.WithStaleTaskTimeout(time.Second))
			var runs atomic.Int32
			require.NoError(t, s.Register(t.Context(), corescheduler.TaskConfig{
				ID: "task", Schedule: "@every 1h", RunOnStart: true,
				Func: func(context.Context) error { runs.Add(1); return nil },
			}))

			// Once the claim response is back, the ownership confirmation and the
			// body (if any) run in the dispatch goroutine, which Stop waits for.
			require.Eventually(t, store.claimed.Load, 5*time.Second, 10*time.Millisecond)
			stopCtx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 10*time.Second)
			defer cancel()
			require.NoError(t, s.Stop(stopCtx))
			require.Equal(t, tc.wantRuns, runs.Load())
			if tc.wantRuns > 0 {
				got, err := mem.GetTask(t.Context(), "task")
				require.NoError(t, err)
				require.Greater(t, got.LastRunAt, int64(0), "the owned run must finish normally")
			}
		})
	}
}

// TestStaleRegistrationDoesNotStarveOtherTasks gives the only dynamic slot to
// nothing but runnable work: a high-priority task whose stored occurrence was
// re-registered elsewhere must not be dispatched, tick after tick, in front of
// a runnable normal-priority task.
func TestStaleRegistrationDoesNotStarveOtherTasks(t *testing.T) {
	t.Parallel()
	mem := mustNewMemory(t, 10)
	leader, follower := &leaderSwitch{}, &leaderSwitch{}
	stale := startForRecovery(t, mem, scheduler.WithTickInterval(20*time.Millisecond), scheduler.WithLeaderElector(leader),
		scheduler.WithConcurrencyLimitFunc(func() int { return 1 }), scheduler.WithReservedHighPrioritySlots(0))
	current := startForRecovery(t, mem, scheduler.WithTickInterval(20*time.Millisecond), scheduler.WithLeaderElector(follower))
	runAt := time.Now().Add(-time.Minute)

	require.NoError(t, stale.Register(t.Context(), corescheduler.TaskConfig{
		ID: "stale", RunAt: runAt, Priority: corescheduler.TaskPriorityHigh, Func: func(context.Context) error { return nil },
	}))
	require.NoError(t, current.Register(t.Context(), corescheduler.TaskConfig{
		ID: "stale", RunAt: runAt.Add(time.Second), Func: func(context.Context) error { return nil },
	}))
	var runs atomic.Int32
	require.NoError(t, stale.Register(t.Context(), corescheduler.TaskConfig{
		ID: "runnable", Schedule: "@every 1h", RunOnStart: true, Priority: corescheduler.TaskPriorityNormal,
		Func: func(context.Context) error { runs.Add(1); return nil },
	}))

	leader.on.Store(true)
	require.Eventually(t, func() bool { return runs.Load() == 1 }, 3*time.Second, 10*time.Millisecond,
		"a stale registration starved a runnable task")
}

// TestUnfinishedRunDoesNotStarveOtherTasks resumes a task whose run is still
// executing on another instance: it is due again but unclaimable, and must not
// take the only dynamic slot of this instance from a runnable task.
func TestUnfinishedRunDoesNotStarveOtherTasks(t *testing.T) {
	t.Parallel()
	mem := mustNewMemory(t, 10)
	cfgFor := func(id string, priority corescheduler.TaskPriority, fn func(context.Context) error) corescheduler.TaskConfig {
		return corescheduler.TaskConfig{ID: id, Schedule: "@every 1s", RunOnStart: true, Priority: priority, Func: fn}
	}
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	var remoteRuns atomic.Int32
	remoteFn := func(ctx context.Context) error {
		remoteRuns.Add(1)
		select {
		case <-release:
		case <-ctx.Done():
		}
		return nil
	}

	remote := startForRecovery(t, mem, scheduler.WithTickInterval(20*time.Millisecond))
	require.NoError(t, remote.Register(t.Context(), cfgFor("long", corescheduler.TaskPriorityHigh, remoteFn)))
	require.Eventually(t, func() bool { return remoteRuns.Load() == 1 }, 2*time.Second, 10*time.Millisecond)
	require.NoError(t, remote.PauseTask(t.Context(), "long"))
	require.NoError(t, remote.ResumeTask(t.Context(), "long"))

	var runs atomic.Int32
	local := startForRecovery(t, mem, scheduler.WithTickInterval(20*time.Millisecond),
		scheduler.WithConcurrencyLimitFunc(func() int { return 1 }), scheduler.WithReservedHighPrioritySlots(0))
	require.NoError(t, local.Register(t.Context(), cfgFor("long", corescheduler.TaskPriorityHigh, remoteFn)))
	// Wait until the resumed task is due (and unclaimable) on every tick.
	require.Eventually(t, func() bool {
		st, err := mem.GetTask(t.Context(), "long")
		return err == nil && st.Status == scheduler.TaskStatusActive && st.NextRunAt < time.Now().Unix()
	}, 3*time.Second, 10*time.Millisecond)
	require.NoError(t, local.Register(t.Context(), cfgFor("runnable", corescheduler.TaskPriorityNormal,
		func(context.Context) error { runs.Add(1); return nil })))

	require.Eventually(t, func() bool { return runs.Load() >= 1 }, 3*time.Second, 10*time.Millisecond,
		"an unclaimable task starved a runnable one")
}

// TestMaximumStaleTimeoutLease checks that the largest stale timeout still
// yields a lease in the future, so another instance leaves the run alone.
func TestMaximumStaleTimeoutLease(t *testing.T) {
	t.Parallel()
	mem := mustNewMemory(t, 10)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	owner := startForRecovery(t, mem, scheduler.WithTickInterval(20*time.Millisecond),
		scheduler.WithStaleTaskTimeout(time.Duration(math.MaxInt64)))
	require.NoError(t, owner.Register(t.Context(), corescheduler.TaskConfig{
		ID: "task", RunAt: time.Now(),
		Func: func(ctx context.Context) error {
			select {
			case <-release:
			case <-ctx.Done():
			}
			return nil
		},
	}))
	require.Eventually(t, func() bool {
		st, err := mem.GetTask(t.Context(), "task")
		return err == nil && st.Status == scheduler.TaskStatusRunning
	}, 2*time.Second, 10*time.Millisecond)
	st, err := mem.GetTask(t.Context(), "task")
	require.NoError(t, err)
	require.Greater(t, st.RunLeaseUntil, time.Now().Unix())

	startForRecovery(t, mem)
	requireStillRunning(t, mem, st.LastRunID)
}

// TestZeroNextRunIsClaimedOnce checks that an active task stored with a zero
// NextRunAt, kept by a registration with the same schedule, is due: it is
// claimed without an occurrence fence, runs once, and its finished run stores
// the next occurrence instead of leaving it due on every tick.
func TestZeroNextRunIsClaimedOnce(t *testing.T) {
	t.Parallel()
	mem := mustNewMemory(t, 10)
	require.NoError(t, mem.UpsertTask(t.Context(), &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{
		ID: "zero", Status: scheduler.TaskStatusActive, Schedule: "@every 1h",
	}}))

	var runs atomic.Int32
	s := startForRecovery(t, mem, scheduler.WithTickInterval(20*time.Millisecond))
	require.NoError(t, s.Register(t.Context(), corescheduler.TaskConfig{
		ID: "zero", Schedule: "@every 1h", Func: func(context.Context) error { runs.Add(1); return nil },
	}))

	require.Eventually(t, func() bool { return runs.Load() == 1 }, 2*time.Second, 10*time.Millisecond,
		"an active task with a zero NextRunAt never ran")
	require.Never(t, func() bool { return runs.Load() > 1 }, 200*time.Millisecond, 10*time.Millisecond)
	st, err := mem.GetTask(t.Context(), "zero")
	require.NoError(t, err)
	require.Equal(t, scheduler.TaskStatusActive, st.Status)
	require.Greater(t, st.NextRunAt, time.Now().Unix())
}
