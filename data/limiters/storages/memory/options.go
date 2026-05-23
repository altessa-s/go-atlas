// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=options

import (
	"time"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

const (
	// DefaultMaxIdleTime is the default maximum idle time before an entry is removed.
	DefaultMaxIdleTime = 30 * time.Minute
)

// options contains memory provider configuration.
type options struct {
	// MaxIdleTime sets the maximum idle time before an entry is removed.
	// Default is 30 minutes.
	maxIdleTime     time.Duration `optgen:"default=DefaultMaxIdleTime" optcheck:"nonzero"`
	cleanupSchedule string
	scheduler       corescheduler.TaskRegistrar `optgen:"notnil"`
}
