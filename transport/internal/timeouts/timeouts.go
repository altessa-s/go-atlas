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

// Config contains timeout settings for server lifecycle operations.
// A zero-value Config is not useful; use [Default] to obtain
// production-ready values.
type Config struct {
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

// Default returns a [Config] with production-ready timeout values:
// 2 s startup verification, 60 s graceful shutdown, and 15 s warning
// interval.
func Default() Config {
	return Config{
		StartupVerification: DefaultStartupVerification,
		ShutdownGraceful:    DefaultShutdownGraceful,
		ShutdownWarning:     DefaultShutdownWarning,
	}
}
