// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

import (
	"log/slog"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// options contains Factory configuration.
type options struct {
	// logger is the logger for operational visibility.
	logger *slog.Logger
	// scheduler is the task scheduler for background processes.
	scheduler corescheduler.TaskRegistrar `optgen:"notnil"`
}
