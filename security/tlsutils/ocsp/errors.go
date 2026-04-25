// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package ocsp

import "errors"

// Sentinel errors for the ocsp package.
var (
	// ErrSchedulerManaged indicates that the function is managed by a scheduler
	// and direct calls are not allowed.
	ErrSchedulerManaged = errors.New("function is managed by scheduler, direct calls not allowed")
)
