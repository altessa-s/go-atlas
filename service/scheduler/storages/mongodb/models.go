// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongodb

import (
	"github.com/altessa-s/go-atlas/domain/converter"
	"github.com/altessa-s/go-atlas/service/scheduler"
)

// taskFieldMapping maps CEL field names (camelCase) to MongoDB BSON field names.
var taskFieldMapping = map[string]string{
	"id": "_id", "lastRunAt": "last_run_at", "nextRunAt": "next_run_at",
	"lastRunId": "last_run_id", "skipNextRun": "skip_next_run",
	"disableHistory": "disable_history", "oneShot": "one_shot",
	"createdAt": "created_at", "updatedAt": "updated_at",
}

// historyFieldMapping maps CEL field names (camelCase) to MongoDB BSON field names.
var historyFieldMapping = map[string]string{
	"id": "_id", "taskId": "task_id", "runId": "run_id",
	"startedAt": "started_at", "endedAt": "ended_at", "durationMs": "duration_ms",
}

// taskDocument represents a task state stored in MongoDB.
type taskDocument struct {
	ID             string            `bson:"_id"`
	Description    string            `bson:"description,omitempty"`
	Status         int32             `bson:"status"`
	Priority       int32             `bson:"priority"`
	Schedule       string            `bson:"schedule"`
	LastRunAt      int64             `bson:"last_run_at,omitempty"`
	NextRunAt      int64             `bson:"next_run_at,omitempty"`
	LastRunID      string            `bson:"last_run_id,omitempty"`
	RunStartedAt   int64             `bson:"run_started_at,omitempty"`
	Failures       int32             `bson:"failures"`
	SkipNextRun    bool              `bson:"skip_next_run,omitempty"`
	DisableHistory bool              `bson:"disable_history,omitempty"`
	Unmanaged      bool              `bson:"unmanaged,omitempty"`
	OneShot        bool              `bson:"one_shot,omitempty"`
	Meta           map[string]string `bson:"meta,omitempty"`
	CreatedAt      int64             `bson:"created_at"`
	UpdatedAt      int64             `bson:"updated_at"`
}

// newTaskDocument creates a new taskDocument from a TaskState.
func newTaskDocument(state *scheduler.TaskState) *taskDocument {
	return converter.Convert(state, &taskDocument{})
}

// toTaskState converts a taskDocument to a TaskState.
func (d *taskDocument) toTaskState() *scheduler.TaskState {
	return converter.Convert(d, &scheduler.TaskState{})
}

// historyDocument represents a task execution history entry stored in MongoDB.
type historyDocument struct {
	ID         string `bson:"_id"`
	TaskID     string `bson:"task_id"`
	RunID      string `bson:"run_id"`
	StartedAt  int64  `bson:"started_at"`
	EndedAt    int64  `bson:"ended_at"`
	DurationMs int64  `bson:"duration_ms"`
	Success    bool   `bson:"success"`
	Error      string `bson:"error,omitempty"`
}

// newHistoryDocument creates a new historyDocument from a TaskHistory.
func newHistoryDocument(history *scheduler.TaskHistory) *historyDocument {
	return converter.Convert(history, &historyDocument{})
}

// toTaskHistory converts a historyDocument to a TaskHistory.
func (d *historyDocument) toTaskHistory() *scheduler.TaskHistory {
	return converter.Convert(d, &scheduler.TaskHistory{})
}
