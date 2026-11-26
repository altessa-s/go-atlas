// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

//go:generate go tool optgen generate

import "github.com/altessa-s/go-atlas-core/time/clock"

const (
	// DefaultKeyPrefix is the default prefix for Redis keys.
	DefaultKeyPrefix = "ratelimit:"
)

// options contains Redis provider configuration.
type options struct {
	// KeyPrefix sets the prefix for Redis keys.
	// Default is "ratelimit:".
	keyPrefix string `optgen:"default=DefaultKeyPrefix" optcheck:"nonempty"`

	// WithClock sets the clock for topology verification caching.
	clock clock.Clock `optgen:"default=clock.Real(),notnil"`

	// WithTestClock overrides Redis TIME for window decisions in tests.
	// It is unsafe with more than one limiter replica. Key expiry still runs
	// on the Redis server's clock.
	testClock clock.Clock `optgen:"notnil"`
}
