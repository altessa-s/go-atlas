// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import (
	"time"

	"github.com/altessa-s/go-atlas/core/time/clock"
)

// DefaultKeyPrefix is the default prefix of the generation keys.
const DefaultKeyPrefix = "generation:"

// DefaultMaxStaleness is the default bound on how long a replica may keep
// using a generation it read before re-reading it from Redis.
const DefaultMaxStaleness = time.Second

// options configures a [Store].
type options struct {
	// keyPrefix is prepended to every namespace to form its Redis key.
	keyPrefix string `optgen:"default=DefaultKeyPrefix"`
	// maxStaleness bounds how long a generation read from Redis is reused
	// locally: the staleness bound of the store. Zero reads Redis on every
	// Current call; negative values are ignored.
	maxStaleness time.Duration `optgen:"default=DefaultMaxStaleness" optval:"positive=allow_zero"`
	// clock measures staleness (default clock.Real()); inject a fake clock in
	// tests. Namespaces are seeded from the Redis server clock, not this one.
	clock clock.Clock `optgen:"default=clock.Real(),notnil"`
}
