// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package inprogress

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate --type=options

import (
	"log/slog"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// options contains configuration fields for Manager that can be set via Option functions.
type options struct {
	// logger for warning on failed InProgress calls.
	logger *slog.Logger
	// scheduler is the scheduler for background task registration (optional)
	scheduler corescheduler.TaskRegistrar `optgen:"notnil"`
	// tickSchedule is the cron schedule for heartbeat tick cycle task
	tickSchedule string
}
