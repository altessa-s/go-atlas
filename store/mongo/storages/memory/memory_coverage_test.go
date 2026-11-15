// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	mongohelpers "github.com/altessa-s/go-atlas/store/mongo/internal/testhelpers"
	cursmemory "github.com/altessa-s/go-atlas/store/mongo/storages/memory"
)

// With a scheduler the cleanup cycle belongs to it: RunCleanup becomes a
// no-op and the registered task does the sweep.
func TestStorage_SchedulerOwnsCleanup(t *testing.T) {
	t.Parallel()
	reg := &testhelpers.MockTaskRegistrar{}
	schedule := " @every 1m "
	blank := " "
	s := cursmemory.New(time.Nanosecond,
		cursmemory.WithScheduler(reg),
		cursmemory.WithScheduler(nil),
		cursmemory.WithCleanupSchedule(&schedule),
		cursmemory.WithCleanupSchedule(blank),
		cursmemory.WithCleanupSchedule(&blank),
		cursmemory.WithCleanupSchedule((*string)(nil)),
	)
	t.Cleanup(s.Close)

	tasks := reg.Tasks()
	require.Len(t, tasks, 1)
	require.Equal(t, "@every 1m", tasks[0].Schedule, "the trimmed schedule is registered")

	require.NoError(t, s.Store(t.Context(), "k", mongohelpers.SampleCursorMetadata()))
	s.RunCleanup()
	require.Equal(t, 1, s.Len(), "a scheduler-managed storage ignores manual cleanup")

	require.NoError(t, tasks[0].Func(t.Context()))
	require.Zero(t, s.Len(), "the scheduled cycle removes expired cursors")
}

func TestStorage_NoScheduleRegistersNothing(t *testing.T) {
	t.Parallel()
	reg := &testhelpers.MockTaskRegistrar{}
	s := cursmemory.New(time.Hour, cursmemory.WithScheduler(reg), cursmemory.WithCleanupSchedule(""))
	require.Empty(t, reg.Tasks())
	s.Close()
}
