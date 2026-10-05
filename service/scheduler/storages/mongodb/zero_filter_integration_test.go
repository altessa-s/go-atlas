// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongodb_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/service/scheduler"
	"github.com/altessa-s/go-atlas/service/scheduler/storages/mongodb"
)

// TestIntegration_MongoZeroFieldsStoredOrOmitted stores the same zero-valued
// task twice — once through the storage, which omits the zero fields, and
// once as a raw document carrying them explicitly, as $set updates and
// earlier releases write them — and requires every filter to select both or
// neither. The two decode to the same state, so no filter may tell them apart.
func TestIntegration_MongoZeroFieldsStoredOrOmitted(t *testing.T) {
	t.Parallel()
	s, db := newClaimITDB(t)
	ctx := t.Context()

	require.NoError(t, s.UpsertTask(ctx, &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{
		ID: "omitted", Status: scheduler.TaskStatusActive,
	}}))
	_, err := db.Collection(mongodb.DefaultTasksCollection).InsertOne(ctx, bson.M{
		"_id": "explicit", "description": "", "status": int32(scheduler.TaskStatusActive), "priority": int32(0),
		"schedule": "", "last_run_at": int64(0), "next_run_at": int64(0), "failures": int32(0),
		"skip_next_run": false, "disable_history": false, "unmanaged": false, "one_shot": false,
		"created_at": int64(0), "updated_at": int64(0),
	})
	require.NoError(t, err)

	parser, err := filter.NewParser()
	require.NoError(t, err)
	for _, expr := range []string{
		`description == ""`, `description != ""`, `!(description == "")`, `description < "a"`, `description > ""`,
		`description == 0`, `description != 0`, `description in [0]`, `description in [0, ""]`,
		`description == null`, `description != null`,
		`description.size() == 0`, `description.size() != 0`, `!(description.size() < 1)`,
		`description.contains("")`, `description.contains("x")`, `description.matches("^$")`, `description.endsWith("a ")`,
		`nextRunAt == 0`, `nextRunAt != 0`, `nextRunAt < 0.5`, `nextRunAt >= 1`, `nextRunAt == ""`, `lastRunAt in [0, 1]`,
		`oneShot == false`, `oneShot`, `!oneShot`, `oneShot != false`, `skipNextRun == false && unmanaged == false`,
		`disableHistory == true || lastRunAt > 0`,
	} {
		node, err := parser.Parse(ctx, expr)
		require.NoError(t, err, expr)
		page, err := s.TasksPaginated(ctx, scheduler.Pagination{Limit: 10}, node)
		require.NoError(t, err, expr)
		require.NotEqual(t, 1, len(page), "%s told the stored and the omitted zero apart", expr)
	}

	require.NoError(t, s.AddHistory(ctx, &scheduler.TaskHistory{ID: "h-omitted", TaskID: "omitted", StartedAt: 1, EndedAt: 2}))
	_, err = db.Collection(mongodb.DefaultHistoryCollection).InsertOne(ctx, bson.M{
		"_id": "h-explicit", "task_id": "omitted", "run_id": "", "error": "",
		"started_at": int64(1), "ended_at": int64(2), "duration_ms": int64(0), "success": false,
	})
	require.NoError(t, err)
	for _, expr := range []string{`error == ""`, `error != ""`, `error.size() == 0`, `error.contains("boom")`, `error != 0`} {
		node, err := parser.Parse(ctx, expr)
		require.NoError(t, err, expr)
		page, err := s.HistoryPaginated(ctx, "omitted",
			scheduler.HistoryPagination{Pagination: scheduler.Pagination{Limit: 10}}, node)
		require.NoError(t, err, expr)
		require.NotEqual(t, 1, len(page), "%s told the stored and the omitted zero apart", expr)
	}
}
