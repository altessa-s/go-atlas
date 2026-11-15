// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package timeouts

import "time"

const (
	// DefaultStartupVerification duration to wait for server startup confirmation.
	DefaultStartupVerification = 2 * time.Second
	// DefaultShutdownGraceful maximum duration to wait for graceful shutdown.
	DefaultShutdownGraceful = time.Minute
	// DefaultShutdownWarning interval for logging shutdown progress warnings.
	DefaultShutdownWarning = 15 * time.Second
)

// Lifecycle is the set of timeouts for server lifecycle operations, carried from
// the server packages to the base server as one value.
// A zero-value Lifecycle is not useful; use [Default] to obtain
// production-ready values.
type Lifecycle struct {
	// StartupVerification is how long [server.BaseServer.Start] waits
	// for the serving goroutine to report an error before declaring
	// startup successful.
	StartupVerification time.Duration

	// ShutdownGraceful is the maximum time allowed for in-flight
	// requests to complete during [server.BaseServer.GracefulShutdown].
	ShutdownGraceful time.Duration

	// ShutdownWarning controls how often a "still shutting down" warning
	// is logged while graceful shutdown is in progress.
	ShutdownWarning time.Duration
}

// Default returns a [Lifecycle] with production-ready timeout values:
// 2 s startup verification, 60 s graceful shutdown, and 15 s warning
// interval.
func Default() Lifecycle {
	return Lifecycle{
		StartupVerification: DefaultStartupVerification,
		ShutdownGraceful:    DefaultShutdownGraceful,
		ShutdownWarning:     DefaultShutdownWarning,
	}
}
