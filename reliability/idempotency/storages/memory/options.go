// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

//go:generate go tool optgen generate

import (
	"time"

	schedtask "github.com/altessa-s/go-horai/task"
)

// DefaultTTL is the default time-to-live for stored keys (24 hours).
const DefaultTTL = 24 * time.Hour

// DefaultCleanupTaskID is the default identifier for the scheduled cleanup.
const DefaultCleanupTaskID = "idempotency-memory-cleanup"

// options contains memory storage configuration.
type options struct {
	ttl             time.Duration `optgen:"default=DefaultTTL"`
	cleanupSchedule string
	cleanupTaskID   string              `optgen:"default=DefaultCleanupTaskID,notblank"`
	clock           func() time.Time    `optgen:"default=time.Now,notnil"`
	scheduler       schedtask.Registrar `optgen:"notnil"`
}
