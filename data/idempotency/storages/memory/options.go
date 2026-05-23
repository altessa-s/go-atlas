// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import (
	"time"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// DefaultTTL is the default time-to-live for stored keys (24 hours).
const DefaultTTL = 24 * time.Hour

// options contains memory storage configuration.
type options struct {
	ttl             time.Duration `optgen:"default=DefaultTTL"`
	cleanupSchedule string
	scheduler       corescheduler.TaskRegistrar `optgen:"notnil"`
}
