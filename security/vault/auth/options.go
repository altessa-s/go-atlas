// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

//go:generate go tool optgen generate

import (
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas/observability/metrics"
)

// Default configuration for authentication retries.
const (
	// DefaultBackoffBase is the delay after the first failed login. It
	// doubles with every consecutive failure.
	DefaultBackoffBase = 2 * time.Second
	// DefaultBackoffMax caps the delay between failed logins.
	DefaultBackoffMax = 30 * time.Second
	// DefaultAuthTimeout is the timeout for individual authentication operations.
	DefaultAuthTimeout = 30 * time.Second
)

type options struct {
	logger      *slog.Logger
	backoffBase time.Duration `optgen:"default=DefaultBackoffBase"`
	backoffMax  time.Duration `optgen:"default=DefaultBackoffMax"`
	authTimeout time.Duration `optgen:"default=DefaultAuthTimeout"`
	// meter ([WithMeter]) records Vault auth metrics. When nil (the default),
	// [metrics.Noop] is used and all metric operations become zero-cost no-ops.
	meter metrics.Meter
	// keepMethodOnExit leaves the auth [Method] usable when
	// [Authenticator.Run] returns instead of calling its Shutdown, which zeroes
	// the credentials. Set it when the method outlives one run, and shut the
	// method down yourself once it is retired.
	keepMethodOnExit bool
	// onLogin is called by [Authenticator.Run], on its goroutine, each time a
	// login succeeded and before Run does anything else with the token. It
	// must not block.
	onLogin func()
}
