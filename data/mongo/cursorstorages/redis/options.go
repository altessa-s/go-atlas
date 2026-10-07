// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import "time"

const (
	// DefaultTTL is the default time-to-live for cursors (1 hour).
	// This is a reasonable default for most pagination use cases.
	DefaultTTL = 1 * time.Hour

	// DefaultKeyPrefix is the default prefix for Redis keys.
	// This prevents key collisions with other data in Redis.
	DefaultKeyPrefix = "cursor:"
)

// options contains Redis cursor storage configuration.
type options struct {
	// TTL sets the time-to-live for cursors in Redis.
	// Cursors will automatically expire after this duration.
	// Default is 1 hour.
	ttl time.Duration `optgen:"default=DefaultTTL"`

	// KeyPrefix sets the prefix for Redis keys.
	// This is useful for namespacing cursors in shared Redis instances.
	// Default is "cursor:".
	keyPrefix string `optgen:"default=DefaultKeyPrefix"`
}
