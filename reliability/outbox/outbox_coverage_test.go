// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox_test

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/runtime/cycle"
	"github.com/altessa-s/go-atlas/reliability/outbox"

	schedtask "github.com/altessa-s/go-horai/task"
)

// batchStore hands out one fixed batch and records every write-back.
type batchStore struct {
	mu        sync.Mutex
	batch     []outbox.Event
	updated   []outbox.Event
	deletedAt []time.Duration
	unlockAt  []time.Duration
}

func (s *batchStore) FetchUnprocessedEvents(context.Context, uint32) ([]outbox.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.batch
	s.batch = nil
	return b, nil
}

func (s *batchStore) DeleteProcessedEvents(_ context.Context, olderThan time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deletedAt = append(s.deletedAt, olderThan)
	return nil
}

func (s *batchStore) UnlockStuckEvents(_ context.Context, lockExpiry time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unlockAt = append(s.unlockAt, lockExpiry)
	return nil
}

func (s *batchStore) SaveEvents(context.Context, ...outbox.Event) error { return nil }

func (s *batchStore) UpdateEvents(_ context.Context, events ...outbox.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updated = append(s.updated, events...)
	return nil
}

func (s *batchStore) ExpireEvents(context.Context) (int64, error) { return 0, nil }

func (s *batchStore) Stats(context.Context) (outbox.Stats, error) { return outbox.Stats{}, nil }

func (s *batchStore) statusByID() map[string]outbox.Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]outbox.Status, len(s.updated))
	for _, e := range s.updated {
		out[e.Id] = e.Status
	}
	return out
}

// recordingHandler records the ids of dispatched events.
type recordingHandler struct {
	mu  sync.Mutex
	ids []string
}

func (h *recordingHandler) handle(_ context.Context, e outbox.Event) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.ids = append(h.ids, e.Id)
	return nil
}

func (h *recordingHandler) dispatched() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Sorted(slices.Values(h.ids))
}

func ev(id, key string, age time.Duration) outbox.Event {
	return outbox.Event{Id: id, Key: key, Status: outbox.StatusInProgress, CreatedAt: time.Now().Add(-age)}
}

func TestOutbox_CompactionDispatchesLatestPerKey(t *testing.T) {
	t.Parallel()
	store := &batchStore{batch: []outbox.Event{
		ev("o1-old", "orders.1", 3*time.Second),
		ev("o1-new", "orders.1", time.Second),
		ev("u1", "users.1", time.Second),
	}}
	h := &recordingHandler{}
	o := outbox.New(store, h.handle, outbox.WithCompaction(), outbox.WithLogger(slog.New(slog.DiscardHandler)))

	require.NoError(t, o.RunDispatchCycle(t.Context()))
	require.Equal(t, []string{"o1-new", "u1"}, h.dispatched())
	status := store.statusByID()
	require.Equal(t, outbox.StatusSkipped, status["o1-old"])
	require.Equal(t, outbox.StatusSent, status["o1-new"])
	require.Equal(t, outbox.StatusSent, status["u1"])
}

func TestOutbox_CompactionFilterLimitsKeys(t *testing.T) {
	t.Parallel()
	store := &batchStore{batch: []outbox.Event{
		ev("o-old", "orders.1", 2*time.Second),
		ev("o-new", "orders.1", time.Second),
		ev("u-old", "users.1", 2*time.Second),
		ev("u-new", "users.1", time.Second),
	}}
	h := &recordingHandler{}
	o := outbox.New(store, h.handle, outbox.WithCompactionFilter(func(k string) bool { return strings.HasPrefix(k, "orders.") }))

	require.NoError(t, o.RunDispatchCycle(t.Context()))
	require.Equal(t, []string{"o-new", "u-new", "u-old"}, h.dispatched(), "only filtered keys are compacted")
	require.Equal(t, outbox.StatusSkipped, store.statusByID()["o-old"])
}

func TestOutbox_CleanupHonorsPublishedLifetime(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		lifetime time.Duration
		want     []time.Duration
	}{
		{name: "retained lifetime", lifetime: time.Hour, want: []time.Duration{time.Hour}},
		{name: "below minimum is ignored", lifetime: time.Second, want: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := &batchStore{}
			o := outbox.New(store, (&recordingHandler{}).handle, outbox.WithPublishedEventsLifetime(tc.lifetime))
			require.NoError(t, o.RunCleanupCycle(t.Context()))
			require.Equal(t, tc.want, store.deletedAt)
		})
	}
}

func TestOutbox_SchedulerFuncsTakeOverManualRuns(t *testing.T) {
	t.Parallel()
	store := &batchStore{batch: []outbox.Event{ev("e1", "k", 0)}}
	h := &recordingHandler{}
	o := outbox.New(store, h.handle,
		outbox.WithPublishedEventsLifetime(time.Hour),
		outbox.WithMaxLockTime(time.Minute),
		outbox.WithHandleTimeout(time.Second),
		outbox.WithUpdateTimeout(time.Second),
		outbox.WithContext(t.Context()))

	dispatch := o.RegisterDispatchSchedulerFunc()
	unlock := o.RegisterUnlockSchedulerFunc()
	cleanup := o.RegisterCleanupSchedulerFunc()

	ctx := t.Context()
	require.ErrorIs(t, o.RunDispatchCycle(ctx), cycle.ErrManaged)
	require.ErrorIs(t, o.RunUnlockCycle(ctx), cycle.ErrManaged)
	require.ErrorIs(t, o.RunCleanupCycle(ctx), cycle.ErrManaged)

	require.NoError(t, dispatch(ctx))
	require.NoError(t, unlock(ctx))
	require.NoError(t, cleanup(ctx))
	require.Equal(t, []string{"e1"}, h.dispatched())
	require.Equal(t, []time.Duration{time.Minute}, store.unlockAt)
	require.Equal(t, []time.Duration{time.Hour}, store.deletedAt)
}

type taskRecorder struct {
	mu    sync.Mutex
	tasks map[string]schedtask.Config
}

func (r *taskRecorder) Register(_ context.Context, cfg schedtask.Config) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.tasks == nil {
		r.tasks = map[string]schedtask.Config{}
	}
	r.tasks[cfg.ID] = cfg
	return nil
}

func TestOutbox_RegisterTasksUsesConfiguredSchedulesAndIDs(t *testing.T) {
	t.Parallel()
	sched := &taskRecorder{}
	dispatchID := " my-dispatch "
	blank := " "
	o := outbox.New(&batchStore{}, (&recordingHandler{}).handle,
		outbox.WithScheduler(sched),
		outbox.WithPublishedEventsLifetime(time.Hour),
		outbox.WithDefaultEventTTL(time.Hour),
		outbox.WithDispatchSchedule(new("@every 1s")), outbox.WithDispatchSchedule(blank), outbox.WithDispatchSchedule((*string)(nil)),
		outbox.WithDispatchTaskID(&dispatchID), outbox.WithDispatchTaskID(&blank),
		outbox.WithUnlockSchedule("@every 2s"), outbox.WithUnlockSchedule(&blank),
		outbox.WithUnlockTaskID("my-unlock"), outbox.WithUnlockTaskID((*string)(nil)),
		outbox.WithCleanupSchedule("@every 3s"), outbox.WithCleanupSchedule(new("")), outbox.WithCleanupSchedule((*string)(nil)),
		outbox.WithCleanupTaskID(new("my-cleanup")), outbox.WithCleanupTaskID(blank), outbox.WithCleanupTaskID((*string)(nil)),
		outbox.WithExpireSchedule(new("@every 4s")), outbox.WithExpireSchedule(blank), outbox.WithExpireSchedule((*string)(nil)),
		outbox.WithExpireTaskID("my-expire"), outbox.WithExpireTaskID(&blank), outbox.WithExpireTaskID((*string)(nil)),
		outbox.WithStatsSchedule(new("@every 5s")), outbox.WithStatsSchedule(""),
		outbox.WithStatsTaskID(new("my-stats")), outbox.WithStatsTaskID(blank), outbox.WithStatsTaskID((*string)(nil)),
	)
	require.NoError(t, o.RegisterTasks(t.Context()))

	want := map[string]string{
		"my-dispatch": "@every 1s",
		"my-unlock":   "@every 2s",
		"my-cleanup":  "@every 3s",
		"my-expire":   "@every 4s",
		"my-stats":    "@every 5s",
	}
	got := make(map[string]string, len(sched.tasks))
	for id, task := range sched.tasks {
		got[id] = task.Schedule
	}
	require.Equal(t, want, got)
	require.False(t, sched.tasks["my-cleanup"].Unmanaged, "cleanup is the only operator-managed task")
	require.True(t, sched.tasks["my-dispatch"].Unmanaged)
}
