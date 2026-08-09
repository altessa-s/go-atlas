// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// capturingRegistrar records every TaskConfig passed to Register so
// tests can assert exactly which IDs the outbox handed to the
// scheduler. It is concurrent-safe because registerTasks could in
// principle be invoked from multiple Outbox instances at once in a
// real deployment (though our tests are single-goroutine).
type capturingRegistrar struct {
	mu         sync.Mutex
	registered []corescheduler.TaskConfig
}

func (r *capturingRegistrar) Register(_ context.Context, cfg corescheduler.TaskConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.registered = append(r.registered, cfg)
	return nil
}

func (r *capturingRegistrar) ids() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.registered))
	for _, c := range r.registered {
		out = append(out, c.ID)
	}
	return out
}

// TestDefaultTaskIDs_MatchLegacyHardcodedValues is the regression
// guard against constant drift. Before PR #41 these IDs were
// "outbox-dispatch" / "outbox-unlock" / "outbox-expire" /
// "outbox-cleanup" hardcoded in scheduler.go. The PR moved them to
// constants — a future rename of the constant would silently break
// any deployment that pinned task-history rows / alert rules to the
// old string. Locking the constant values down at the test level
// makes a rename a deliberate, reviewer-visible decision.
func TestDefaultTaskIDs_MatchLegacyHardcodedValues(t *testing.T) {
	t.Parallel()

	require.Equal(t, "outbox-dispatch", DefaultDispatchTaskID)
	require.Equal(t, "outbox-unlock", DefaultUnlockTaskID)
	require.Equal(t, "outbox-expire", DefaultExpireTaskID)
	require.Equal(t, "outbox-cleanup", DefaultCleanupTaskID)
	require.Equal(t, "outbox-stats", DefaultStatsTaskID)
}

// failingRegistrar rejects every registration, standing in for an invalid cron
// expression or an ID the scheduler refuses.
type failingRegistrar struct{ err error }

func (r *failingRegistrar) Register(context.Context, corescheduler.TaskConfig) error { return r.err }

// A rejected registration must leave the manual entry point usable. Marking a
// cycle scheduler-managed before Register succeeds would strand it: nothing
// drives it automatically, and RunDispatchCycle answers ErrSchedulerManaged.
// New only logs the registration failure, so the outbox would silently stop
// delivering with no way to run a cycle by hand.
func TestRegisterTasks_FailedRegistrationKeepsManualRunUsable(t *testing.T) {
	t.Parallel()

	ob := New(&recordingStore{}, noopHandler,
		WithScheduler(&failingRegistrar{err: errors.New("invalid cron expression")}),
		WithDispatchSchedule("@every 1s"),
		WithUnlockSchedule("@every 11s"),
		WithStatsSchedule("@every 30s"),
	)

	require.NoError(t, ob.RunDispatchCycle(t.Context()))
	require.NoError(t, ob.RunUnlockCycle(t.Context()))
	require.NoError(t, ob.RunStatsCycle(t.Context()))
}

// The mirror image: once registration succeeds, the scheduler owns the cycle
// and manual invocation must be refused so the two cannot overlap.
func TestRegisterTasks_SuccessfulRegistrationBlocksManualRun(t *testing.T) {
	t.Parallel()

	ob := New(&recordingStore{}, noopHandler,
		WithScheduler(&capturingRegistrar{}),
		WithDispatchSchedule("@every 1s"),
		WithStatsSchedule("@every 30s"),
	)

	require.ErrorIs(t, ob.RunDispatchCycle(t.Context()), corescheduler.ErrSchedulerManaged)
	require.ErrorIs(t, ob.RunStatsCycle(t.Context()), corescheduler.ErrSchedulerManaged)
	// Never scheduled — no schedule was configured — so it stays manual.
	require.NoError(t, ob.RunUnlockCycle(t.Context()))
}

// Each registered cycle must be driven by its own guarded body. A table-driven
// registration loop is easy to get wrong by capturing one iteration variable
// for every task, which would point every entry at the same cycle.
func TestRegisterTasks_EachTaskRunsItsOwnCycle(t *testing.T) {
	t.Parallel()

	reg := &capturingRegistrar{}
	store := &statsStore{stats: Stats{Pending: 1}}
	ob := New(store, noopHandler,
		WithScheduler(reg),
		WithDispatchSchedule("@every 1s"),
		WithUnlockSchedule("@every 11s"),
		WithStatsSchedule("@every 30s"),
	)
	_ = ob

	reg.mu.Lock()
	registered := append([]corescheduler.TaskConfig(nil), reg.registered...)
	reg.mu.Unlock()
	require.Len(t, registered, 3)

	for _, cfg := range registered {
		require.NotNil(t, cfg.Func, "task %q must carry a body", cfg.ID)
		require.NoError(t, cfg.Func(t.Context()), "task %q must be runnable", cfg.ID)
	}

	require.Positive(t, store.fetched.Load(), "the dispatch task must drive the dispatch cycle")
	require.Positive(t, store.unlocked.Load(), "the unlock task must drive the unlock cycle")
	require.Positive(t, store.statsCalls.Load(), "the stats task must drive the stats cycle")
}

// TestRegisterTasks_OverridesPropagateToScheduler proves the end-to-end
// wire-up: a WithDispatchTaskID / WithUnlockTaskID override threaded
// through New() reaches TaskConfig.ID on the scheduler.Register call.
// This is the "the new option actually works" guard — without it, a
// regression in scheduler.go that quietly reverts to a hardcoded
// string would go undetected because the rest of the package never
// reads opts.dispatchTaskID.
func TestRegisterTasks_OverridesPropagateToScheduler(t *testing.T) {
	t.Parallel()

	reg := &capturingRegistrar{}
	_ = New(nil, nil,
		WithScheduler(reg),
		WithDispatchSchedule("@every 1s"),
		WithUnlockSchedule("@every 11s"),
		WithDispatchTaskID("svc-a-outbox-dispatch"),
		WithUnlockTaskID("svc-a-outbox-unlock"),
	)

	ids := reg.ids()
	require.Contains(t, ids, "svc-a-outbox-dispatch",
		"WithDispatchTaskID override must reach scheduler.Register")
	require.Contains(t, ids, "svc-a-outbox-unlock",
		"WithUnlockTaskID override must reach scheduler.Register")
	require.NotContains(t, ids, DefaultDispatchTaskID,
		"the default ID must NOT be registered when an override is set")
}

// TestRegisterTasks_DefaultsRegisterLegacyIDs is the symmetric case
// — without any overrides the scheduler sees the legacy IDs, so
// existing deployments that pinned dashboards or alert rules to
// "outbox-dispatch" keep working after PR #41 lands.
func TestRegisterTasks_DefaultsRegisterLegacyIDs(t *testing.T) {
	t.Parallel()

	reg := &capturingRegistrar{}
	_ = New(nil, nil,
		WithScheduler(reg),
		WithDispatchSchedule("@every 1s"),
	)

	require.Equal(t, []string{DefaultDispatchTaskID}, reg.ids())
}

// TestValidateTaskIDs_RejectsCollision pins the collision check —
// two task IDs sharing the same value would let the scheduler upsert
// the second registration over the first one's Func pointer,
// silently disabling one of the cycles. The check has to run BEFORE
// any Register call, otherwise the first task lands in storage and
// has to be cleaned up manually.
func TestValidateTaskIDs_RejectsCollision(t *testing.T) {
	t.Parallel()

	opts := &options{
		dispatchTaskID: "shared",
		unlockTaskID:   "shared",
		expireTaskID:   DefaultExpireTaskID,
		cleanupTaskID:  DefaultCleanupTaskID,
		statsTaskID:    DefaultStatsTaskID,
	}
	err := validateTaskIDs(opts)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrTaskIDCollision),
		"collision must surface as ErrTaskIDCollision so callers can branch on it")
	require.Contains(t, err.Error(), "shared",
		"error must echo the offending value so the operator can grep the YAML")
}

// TestValidateTaskIDs_RejectsEmpty guards the programmatic-misuse
// path: an empty task ID would make scheduler.Register fail later
// with the generic "task ID cannot be empty" message, but by then
// some of the other tasks may already be registered. Surface it as
// the same ErrTaskIDCollision class so the runtime always fails at
// the same chokepoint.
func TestValidateTaskIDs_RejectsEmpty(t *testing.T) {
	t.Parallel()

	opts := &options{
		dispatchTaskID: "",
		unlockTaskID:   DefaultUnlockTaskID,
		expireTaskID:   DefaultExpireTaskID,
		cleanupTaskID:  DefaultCleanupTaskID,
		statsTaskID:    DefaultStatsTaskID,
	}
	err := validateTaskIDs(opts)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrTaskIDCollision))
}

// TestValidateTaskIDs_DefaultsAreDistinct is a cheap sanity check:
// the DefaultXxxTaskID constants must never accidentally collide. A
// copy-paste edit that aliased two of them (e.g. setting
// DefaultUnlockTaskID = "outbox-dispatch") would only show up at
// runtime as a registerTasks failure on the second Outbox to start
// up. Catching it here makes the breakage visible at unit-test time.
func TestValidateTaskIDs_DefaultsAreDistinct(t *testing.T) {
	t.Parallel()

	opts := &options{
		dispatchTaskID: DefaultDispatchTaskID,
		unlockTaskID:   DefaultUnlockTaskID,
		expireTaskID:   DefaultExpireTaskID,
		cleanupTaskID:  DefaultCleanupTaskID,
		statsTaskID:    DefaultStatsTaskID,
	}
	require.NoError(t, validateTaskIDs(opts),
		"every Default*TaskID constant must be distinct out of the box")
}
