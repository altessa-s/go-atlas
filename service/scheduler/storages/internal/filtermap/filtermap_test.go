// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filtermap_test

import (
	"maps"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/service/scheduler"
	"github.com/altessa-s/go-atlas/service/scheduler/storages/internal/filtermap"
)

// TestKeysMatchFilterFields pins the maps to the advertised filter fields: a
// field missing from a map is a filter that can never match on the storages
// evaluating on the client.
func TestKeysMatchFilterFields(t *testing.T) {
	t.Parallel()
	require.ElementsMatch(t, scheduler.TaskFilterFields, slices.Collect(maps.Keys(filtermap.Task(&scheduler.TaskState{}))))
	require.ElementsMatch(t, scheduler.HistoryFilterFields, slices.Collect(maps.Keys(filtermap.History(&scheduler.TaskHistory{}))))
}

func TestValues(t *testing.T) {
	t.Parallel()
	task := filtermap.Task(&scheduler.TaskState{TaskSummary: scheduler.TaskSummary{
		ID: "t", Status: scheduler.TaskStatusActive, Priority: 2, Failures: 3, OneShot: true, NextRunAt: 9,
	}})
	require.Equal(t, "t", task["id"])
	require.Equal(t, int64(scheduler.TaskStatusActive), task["status"])
	require.Equal(t, int64(2), task["priority"])
	require.Equal(t, int64(3), task["failures"])
	require.Equal(t, true, task["oneShot"])
	require.Equal(t, int64(9), task["nextRunAt"])

	hist := filtermap.History(&scheduler.TaskHistory{ID: "h", TaskID: "t", StartedAt: 5, Success: true})
	require.Equal(t, "t", hist["taskId"])
	require.Equal(t, int64(5), hist["startedAt"])
	require.Equal(t, true, hist["success"])
}
