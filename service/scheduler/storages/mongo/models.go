// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"reflect"
	"slices"
	"strings"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/domain/converter"
	"github.com/altessa-s/go-atlas/service/scheduler"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
)

// taskFieldMapping maps CEL field names (camelCase) to MongoDB BSON field names.
var taskFieldMapping = coremaps.NewImmutableMap(map[string]string{
	"id": "_id", "lastRunAt": "last_run_at", "nextRunAt": "next_run_at",
	"lastRunId": "last_run_id", "skipNextRun": "skip_next_run",
	"disableHistory": "disable_history", "oneShot": "one_shot",
	"createdAt": "created_at", "updatedAt": "updated_at",
})

// historyFieldMapping maps CEL field names (camelCase) to MongoDB BSON field names.
var historyFieldMapping = coremaps.NewImmutableMap(map[string]string{
	"id": "_id", "taskId": "task_id", "runId": "run_id",
	"startedAt": "started_at", "endedAt": "ended_at", "durationMs": "duration_ms",
})

// taskZeroFields and historyZeroFields declare the fields a document omits
// when they hold their zero value (the omitempty bson tags below), keyed by
// CEL name, for [filter.WithZeroWhenAbsent]: a filter such as
// `description == ""` must select a task stored without a description, as
// every other backend does. They are derived from the document structs so a
// field that becomes omitempty cannot be left out.
var (
	taskZeroFields    = omittedZeroFields(reflect.TypeFor[taskDocument](), taskFieldMapping)
	historyZeroFields = omittedZeroFields(reflect.TypeFor[historyDocument](), historyFieldMapping)
)

// omittedZeroFields returns the omitempty fields of doc whose Go type has a
// filter kind, keyed by CEL name — the inverse of mapping, or the bson name
// itself where mapping has no entry.
func omittedZeroFields(doc reflect.Type, mapping *coremaps.ImmutableMap[string, string]) *coremaps.ImmutableMap[string, filter.FieldKind] {
	celNames := make(map[string]string, mapping.Len())
	for cel, bsonName := range mapping.All() {
		celNames[bsonName] = cel
	}
	fields := make(map[string]filter.FieldKind)
	for f := range doc.Fields() {
		name, opts, _ := strings.Cut(f.Tag.Get("bson"), ",")
		if !slices.Contains(strings.Split(opts, ","), "omitempty") {
			continue
		}
		kind := goFieldKind(f.Type.Kind())
		if kind == filter.FieldKindUnspecified {
			continue
		}
		if cel, ok := celNames[name]; ok {
			name = cel
		}
		fields[name] = kind
	}
	return coremaps.NewImmutableMap(fields)
}

// goFieldKind maps the Go kinds the documents use onto filter kinds.
func goFieldKind(k reflect.Kind) filter.FieldKind {
	switch k {
	case reflect.String:
		return filter.FieldKindString
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return filter.FieldKindInt
	case reflect.Bool:
		return filter.FieldKindBool
	default:
		return filter.FieldKindUnspecified
	}
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
	RunLeaseUntil  int64             `bson:"run_lease_until,omitempty"`
	RunLeaseID     string            `bson:"run_lease_id,omitempty"`
	RunAt          int64             `bson:"run_at,omitempty"`
	Failures       int32             `bson:"failures"`
	SkipNextRun    bool              `bson:"skip_next_run,omitempty"`
	DisableHistory bool              `bson:"disable_history,omitempty"`
	Unmanaged      bool              `bson:"unmanaged,omitempty"`
	OneShot        bool              `bson:"one_shot,omitempty"`
	Meta           map[string]string `bson:"meta,omitempty"`
	CreatedAt      int64             `bson:"created_at"`
	UpdatedAt      int64             `bson:"updated_at"`
	Revision       int64             `bson:"revision,omitempty"`
}

// newTaskDocument creates a new taskDocument from a TaskState.
func newTaskDocument(state *scheduler.TaskState) *taskDocument {
	return converter.Convert(state, &taskDocument{})
}

// toTaskState converts a taskDocument to a TaskState.
func (d *taskDocument) toTaskState() *scheduler.TaskState {
	return converter.Convert(d, &scheduler.TaskState{}, converter.WithHandleEmbeddedStructs(false))
}

// historyDocument represents a task execution history entry stored in MongoDB.
type historyDocument struct {
	ID         string `bson:"_id"`
	TaskID     string `bson:"task_id"`
	RunID      string `bson:"run_id"`
	Error      string `bson:"error,omitempty"`
	StartedAt  int64  `bson:"started_at"`
	EndedAt    int64  `bson:"ended_at"`
	DurationMs int64  `bson:"duration_ms"`
	Success    bool   `bson:"success"`
}

// newHistoryDocument creates a new historyDocument from a TaskHistory.
func newHistoryDocument(history *scheduler.TaskHistory) *historyDocument {
	return converter.Convert(history, &historyDocument{})
}

// toTaskHistory converts a historyDocument to a TaskHistory.
func (d *historyDocument) toTaskHistory() *scheduler.TaskHistory {
	return converter.Convert(d, &scheduler.TaskHistory{})
}
