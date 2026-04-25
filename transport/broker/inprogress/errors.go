// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package inprogress

import "errors"

var (
	// ErrSchedulerManaged is returned by [Manager.RunTickCycle] when the tick
	// function has been registered with a scheduler via
	// [Manager.RegisterTickSchedulerFunc]. Direct calls are not allowed once
	// scheduler management is active.
	ErrSchedulerManaged = errors.New("function is managed by scheduler, direct calls not allowed")
)
