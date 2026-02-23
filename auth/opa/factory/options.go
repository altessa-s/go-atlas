// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate --type=options

import (
	"log/slog"

	"github.com/altessa-s/go-atlas/observability/health"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// options holds the configuration for the OPA factory.
type options struct {
	// logger sets the logger for the factory.
	logger *slog.Logger
	// scheduler sets the scheduler for automatic policy updates.
	// When a scheduler is provided and UpdateSchedule is configured,
	// the manager will register a task for periodic policy updates.
	scheduler corescheduler.TaskRegistrar `optgen:"notnil"`
	// healthCoordinator registers OPA managers with the health coordinator.
	healthCoordinator *health.Coordinator
}
