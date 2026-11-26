// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"
	"github.com/altessa-s/go-atlas/messaging/broker/providers/nats/recovery"

	natsprovider "github.com/altessa-s/go-atlas/messaging/broker/providers/nats"
	schedtask "github.com/altessa-s/go-horai/task"
)

type recordingRegistrar struct {
	mu  sync.Mutex
	ids []string
}

func (r *recordingRegistrar) Register(_ context.Context, cfg schedtask.Config) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ids = append(r.ids, cfg.ID)
	return nil
}

func newTaskManager(t *testing.T, provider *natsprovider.NATS, sched schedtask.Registrar, extra ...recovery.Option) (*recovery.Manager, error) {
	t.Helper()
	opts := append([]recovery.Option{
		recovery.WithScheduler(sched),
		recovery.WithHealthCheckSchedule("@every 1m"),
		recovery.WithStaleRecoveryCleanupSchedule("@every 1m"),
	}, extra...)
	m, err := recovery.New(provider, opts...)
	if err == nil {
		t.Cleanup(m.Close)
	}
	return m, err
}

func newTaskProvider(t *testing.T) *natsprovider.NATS {
	t.Helper()
	ns := natstest.StartNATSServer(t)
	conn, _ := natstest.ConnectJetStream(t, ns)
	provider, err := natsprovider.New(conn)
	require.NoError(t, err)
	return provider
}

// TestManager_DefaultTaskIDsAreUniquePerInstance verifies that managers with
// default options register distinct scheduler task IDs: the scheduler keys
// tasks by ID, so a shared ID leaves only the last manager running.
func TestManager_DefaultTaskIDsAreUniquePerInstance(t *testing.T) {
	t.Parallel()

	provider := newTaskProvider(t)
	reg := &recordingRegistrar{}
	for range 2 {
		_, err := newTaskManager(t, provider, reg)
		require.NoError(t, err)
	}

	require.Len(t, reg.ids, 4)
	unique := map[string]struct{}{}
	for _, id := range reg.ids {
		unique[id] = struct{}{}
	}
	require.Len(t, unique, 4, "default task IDs of two managers must not collide: %v", reg.ids)
}

func TestManager_ExplicitTaskIDs(t *testing.T) {
	t.Parallel()

	provider := newTaskProvider(t)
	reg := &recordingRegistrar{}
	_, err := newTaskManager(t, provider, reg, recovery.WithHealthCheckTaskID("orders-health"), recovery.WithStaleCleanupTaskID("orders-stale"))
	require.NoError(t, err)
	require.Equal(t, []string{"orders-health", "orders-stale"}, reg.ids)
}

// TestManager_EqualTaskIDsAreRejectedBeforeRegistration verifies that equal
// effective IDs, which would let one task replace the other, fail
// construction without registering anything.
func TestManager_EqualTaskIDsAreRejectedBeforeRegistration(t *testing.T) {
	t.Parallel()

	provider := newTaskProvider(t)
	reg := &recordingRegistrar{}
	_, err := newTaskManager(t, provider, reg, recovery.WithHealthCheckTaskID("same"), recovery.WithStaleCleanupTaskID("same"))
	require.ErrorIs(t, err, recovery.ErrDuplicateTaskID)
	require.Empty(t, reg.ids)
}

type failSecondRegistrar struct {
	recordingRegistrar
	unregistered  []string
	canUnregister bool
	funcs         map[string]schedtask.Func
}

var errSecondRejected = errors.New("second registration rejected")

func (r *failSecondRegistrar) Register(ctx context.Context, cfg schedtask.Config) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.ids) == 1 {
		return errSecondRejected
	}
	if r.funcs == nil {
		r.funcs = map[string]schedtask.Func{}
	}
	r.ids = append(r.ids, cfg.ID)
	r.funcs[cfg.ID] = cfg.Func
	return nil
}

type unregisteringRegistrar struct{ *failSecondRegistrar }

func (r unregisteringRegistrar) Unregister(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.unregistered = append(r.unregistered, id)
	return nil
}

// TestManager_PartialRegistrationIsRecoverable verifies that when the second
// registration fails, the first task is unregistered when the scheduler can,
// and otherwise is left inert instead of running a discarded manager.
func TestManager_PartialRegistrationIsRecoverable(t *testing.T) {
	t.Parallel()

	provider := newTaskProvider(t)

	t.Run("scheduler supports unregister", func(t *testing.T) {
		t.Parallel()
		reg := &failSecondRegistrar{}
		_, err := newTaskManager(t, provider, unregisteringRegistrar{reg})
		require.ErrorIs(t, err, errSecondRejected)
		require.Equal(t, reg.ids, reg.unregistered)
	})

	t.Run("scheduler cannot unregister", func(t *testing.T) {
		t.Parallel()
		reg := &failSecondRegistrar{}
		_, err := newTaskManager(t, provider, reg)
		require.ErrorIs(t, err, errSecondRejected)
		require.Len(t, reg.ids, 1)
		require.ErrorIs(t, reg.funcs[reg.ids[0]](t.Context()), recovery.ErrManagerClosed,
			"the orphaned task must be inert")
	})
}
