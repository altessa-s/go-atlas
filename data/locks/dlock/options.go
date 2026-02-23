// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dlock

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

import (
	"log/slog"
	"time"
)

// DefaultLockAcquireTimeout is the default timeout for lock acquisition operations.
// This prevents indefinite blocking in scenarios where locks cannot be acquired.
const DefaultLockAcquireTimeout = 30 * time.Second

// options contains DLock configuration.
type options struct {
	logger             *slog.Logger
	lockAcquireTimeout time.Duration `optgen:"default=DefaultLockAcquireTimeout"`
}
