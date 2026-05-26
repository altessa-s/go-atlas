// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox

import "errors"

// Sentinel errors for the outbox package.
var (
	// ErrSchedulerManaged indicates that the function is managed by a scheduler
	// and direct calls are not allowed.
	ErrSchedulerManaged = errors.New("function is managed by scheduler, direct calls not allowed")

	// ErrTaskIDCollision is returned from registerTasks when two or more of
	// the configured scheduler task IDs (dispatch / unlock / expire /
	// cleanup) are equal. The underlying scheduler upserts by ID, so a
	// collision would silently overwrite the first task's Func pointer
	// with the second's — we surface it as a startup error instead.
	ErrTaskIDCollision = errors.New("outbox scheduler task IDs must be distinct")
)
