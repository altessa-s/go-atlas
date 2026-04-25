// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dlock

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

import (
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// DefaultLockAcquireTimeout is the default timeout for lock acquisition operations.
// This prevents indefinite blocking in scenarios where locks cannot be acquired.
const DefaultLockAcquireTimeout = 30 * time.Second

// DefaultHealthServiceName is the name under which DLock registers itself
// with the health coordinator when no override is supplied.
const DefaultHealthServiceName = "dlock"

// options contains DLock configuration.
type options struct {
	logger             *slog.Logger
	lockAcquireTimeout time.Duration     `optgen:"default=DefaultLockAcquireTimeout"`
	collector          metrics.Collector `optgen:"notnil"`
	// healthCoordinator registers DLock with a health coordinator on
	// construction. Disabled when nil.
	healthCoordinator *health.Coordinator
	// healthServiceName customizes the service name used for health
	// registration. Useful when multiple DLock instances share one
	// coordinator (e.g. "dlock-payments", "dlock-inventory").
	healthServiceName string `optgen:"default=DefaultHealthServiceName"`
}
