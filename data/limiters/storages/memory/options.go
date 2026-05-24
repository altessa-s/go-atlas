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

	// DefaultMaxBuckets caps the number of distinct rate-limit keys held
	// in memory. Without a cap, an attacker who can drive arbitrary keys
	// (e.g. IPv6 source addresses, every value 2^128, through a per-IP
	// limiter) grows the map without bound and OOMs the process — and
	// the [DefaultMaxIdleTime]-based cleanup is opt-in via the scheduler,
	// so by default nothing reclaims memory.
	//
	// 100_000 is enough to track a large rotation of legitimate clients
	// while keeping per-bucket overhead (~few hundred bytes including the
	// requests slice) at single-digit MB. Operators with truly high key
	// cardinality should reach for a Redis-backed limiter — the
	// in-memory provider is for single-process / low-cardinality cases.
	DefaultMaxBuckets = 100_000
)

// options contains memory provider configuration.
type options struct {
	// MaxIdleTime sets the maximum idle time before an entry is removed.
	// Default is 30 minutes.
	maxIdleTime time.Duration `optgen:"default=DefaultMaxIdleTime" optcheck:"nonzero"`

	// MaxBuckets caps the number of distinct keys the provider will hold.
	// When the cap is reached and Allow is called for a new key, the
	// least-recently-used bucket is evicted to make room. Set to <= 0
	// to disable the cap (back to legacy unbounded behavior — use at
	// your own risk; see [DefaultMaxBuckets] for the rationale).
	maxBuckets      int `optgen:"default=DefaultMaxBuckets"`
	cleanupSchedule string
	scheduler       corescheduler.TaskRegistrar `optgen:"notnil"`
}
