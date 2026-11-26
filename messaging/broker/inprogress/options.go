// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package inprogress

//go:generate go tool optgen generate --type=options

import (
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas/observability/metrics"

	schedtask "github.com/altessa-s/go-horai/task"
)

// DefaultHeartbeatTimeout is the default bound of a single InProgress call.
const DefaultHeartbeatTimeout = 5 * time.Second

// options contains configuration fields for Manager that can be set via Option functions.
type options struct {
	// logger for warning on failed InProgress calls.
	logger *slog.Logger
	// scheduler is the scheduler for background task registration (optional)
	scheduler schedtask.Registrar `optgen:"notnil"`
	// tickSchedule is the cron schedule for heartbeat tick cycle task
	tickSchedule string
	// tickTaskID is the scheduler task ID of the heartbeat tick cycle. The
	// scheduler keys tasks by ID, so two managers sharing an ID leave only
	// the last one heartbeating. Empty derives a unique ID per manager
	// instance, "broker-inprogress-heartbeat-<n>"; set it for an ID that is
	// stable across restarts.
	tickTaskID string
	// meter is the metrics meter for instrumentation.
	meter metrics.Meter `optgen:"notnil"`
	// heartbeatTimeout bounds a single InProgress call, so that one slow
	// heartbeater cannot hold a tick past the AckWait of the others. A
	// non-positive value keeps the default.
	heartbeatTimeout time.Duration `optgen:"default=DefaultHeartbeatTimeout"`
}
