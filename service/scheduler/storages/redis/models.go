// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

import (
	"github.com/altessa-s/go-atlas/data/filter/translators/redisearch"
	"github.com/altessa-s/go-atlas/domain/converter"
	"github.com/altessa-s/go-atlas/service/scheduler"
)

// taskFieldSchema maps RediSearch index field names to their types for task queries.
var taskFieldSchema = map[string]redisearch.FieldType{
	"id":             redisearch.FieldTypeTag,
	"description":    redisearch.FieldTypeText,
	"status":         redisearch.FieldTypeNumeric,
	"priority":       redisearch.FieldTypeNumeric,
	"schedule":       redisearch.FieldTypeTag,
	"lastRunAt":      redisearch.FieldTypeNumeric,
	"nextRunAt":      redisearch.FieldTypeNumeric,
	"failures":       redisearch.FieldTypeNumeric,
	"skipNextRun":    redisearch.FieldTypeTag,
	"disableHistory": redisearch.FieldTypeTag,
	"unmanaged":      redisearch.FieldTypeTag,
	"oneShot":        redisearch.FieldTypeTag,
	"createdAt":      redisearch.FieldTypeNumeric,
	"updatedAt":      redisearch.FieldTypeNumeric,
}

// taskFieldMapping maps CEL field names to RediSearch index field names (identity for most).
// This mapping is used when the CEL field name doesn't match the index alias.
var taskFieldMapping = map[string]string{}

// historyFieldSchema maps RediSearch index field names to their types for history queries.
var historyFieldSchema = map[string]redisearch.FieldType{
	"id":         redisearch.FieldTypeTag,
	"taskId":     redisearch.FieldTypeTag,
	"runId":      redisearch.FieldTypeTag,
	"startedAt":  redisearch.FieldTypeNumeric,
	"endedAt":    redisearch.FieldTypeNumeric,
	"durationMs": redisearch.FieldTypeNumeric,
	"success":    redisearch.FieldTypeTag,
	"error":      redisearch.FieldTypeText,
}

// taskData represents a task state stored in Redis as JSON.
type taskData struct {
	ID             string            `json:"id"`
	Description    string            `json:"description,omitempty"`
	Status         int32             `json:"status"`
	Priority       int32             `json:"priority"`
	Schedule       string            `json:"schedule"`
	LastRunAt      int64             `json:"last_run_at,omitempty"`
	NextRunAt      int64             `json:"next_run_at,omitempty"`
	LastRunID      string            `json:"last_run_id,omitempty"`
	RunStartedAt   int64             `json:"run_started_at,omitempty"`
	Failures       int32             `json:"failures"`
	SkipNextRun    bool              `json:"skip_next_run,omitempty"`
	DisableHistory bool              `json:"disable_history,omitempty"`
	Unmanaged      bool              `json:"unmanaged,omitempty"`
	OneShot        bool              `json:"one_shot,omitempty"`
	Meta           map[string]string `json:"meta,omitempty"`
	CreatedAt      int64             `json:"created_at"`
	UpdatedAt      int64             `json:"updated_at"`
}

// newTaskData creates a new taskData from a TaskState.
func newTaskData(state *scheduler.TaskState) *taskData {
	return converter.Convert(state, &taskData{})
}

// toTaskState converts a taskData to a TaskState.
func (d *taskData) toTaskState() *scheduler.TaskState {
	return converter.Convert(d, &scheduler.TaskState{})
}

// historyData represents a task execution history entry stored in Redis as JSON.
type historyData struct {
	ID         string `json:"id"`
	TaskID     string `json:"task_id"`
	RunID      string `json:"run_id"`
	StartedAt  int64  `json:"started_at"`
	EndedAt    int64  `json:"ended_at"`
	DurationMs int64  `json:"duration_ms"`
	Success    bool   `json:"success"`
	Error      string `json:"error,omitempty"`
}

// newHistoryData creates a new historyData from a TaskHistory.
func newHistoryData(history *scheduler.TaskHistory) *historyData {
	return converter.Convert(history, &historyData{})
}

// toTaskHistory converts a historyData to a TaskHistory.
func (d *historyData) toTaskHistory() *scheduler.TaskHistory {
	return converter.Convert(d, &scheduler.TaskHistory{})
}
