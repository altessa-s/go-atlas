// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lifecycle

import (
	"time"

	"github.com/altessa-s/go-atlas/internal/transport/timeouts"
)

// Timeouts controls startup verification and graceful shutdown for HTTP and gRPC servers.
type Timeouts struct {
	// StartupVerification is the interval to observe early Serve failures. Zero skips the wait.
	StartupVerification time.Duration
	// ShutdownGraceful bounds protocol cleanup, subject to any shorter caller deadline.
	// A nonpositive duration uses the default of one minute.
	ShutdownGraceful time.Duration
	// ShutdownWarning is the interval between shutdown progress logs.
	// A nonpositive duration uses the default of 15 seconds.
	ShutdownWarning time.Duration
}

// DefaultTimeouts returns a two-second startup window, one-minute graceful shutdown
// limit and 15-second warning interval.
func DefaultTimeouts() Timeouts { return Timeouts(timeouts.Default()) }
