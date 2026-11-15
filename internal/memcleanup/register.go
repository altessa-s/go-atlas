// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memcleanup

import (
	"context"
	"errors"

	"github.com/altessa-s/go-atlas-core/runtime/cycle"
	"github.com/altessa-s/go-atlas-core/types/nilcheck"

	schedtask "github.com/altessa-s/go-horai/task"
)

var (
	// ErrNoScheduler is returned by [Register] when the registrar is nil.
	ErrNoScheduler = errors.New("scheduler not configured")
	// ErrNoSchedule is returned by [Register] when the task has no schedule.
	ErrNoSchedule = errors.New("cleanup schedule must be set")
)

// Register hands a storage's cleanup cycle to registrar. The registered
// cfg.Func is guard's entry point, so scheduled runs share its overlap guard;
// after a successful registration guard is marked managed, so the storage's
// manual RunCleanup becomes a no-op that reports [cycle.ErrManaged] from
// guard.Run.
//
// It returns [ErrNoScheduler] for a nil registrar and [ErrNoSchedule] for an
// empty cfg.Schedule, leaving guard untouched in both cases, as it does when
// registrar rejects the task.
func Register(
	ctx context.Context,
	registrar schedtask.Registrar,
	guard *cycle.Guard,
	cfg schedtask.Config,
) error {
	if nilcheck.IsNil(registrar) {
		return ErrNoScheduler
	}
	if cfg.Schedule == "" {
		return ErrNoSchedule
	}
	cfg.Func = guard.Entry()
	if err := registrar.Register(ctx, cfg); err != nil {
		return err
	}
	guard.MarkManaged()
	return nil
}
