// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=options

const (
	// DefaultTasksTable is the table holding [scheduler.TaskState] rows when no
	// override is provided via [WithTasksTable].
	DefaultTasksTable = "scheduler_tasks"

	// DefaultHistoryTable is the table holding [scheduler.TaskHistory] rows
	// when no override is provided via [WithHistoryTable].
	DefaultHistoryTable = "scheduler_history"
)

type options struct {
	tasksTable   string `optgen:"default=DefaultTasksTable"`
	historyTable string `optgen:"default=DefaultHistoryTable"`
}
