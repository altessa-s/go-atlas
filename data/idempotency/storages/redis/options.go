// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

import (
	"time"
)

// DefaultTTL is the default time-to-live for keys (24 hours).
const DefaultTTL = 24 * time.Hour

// MinTTL is the minimum allowed TTL (1 minute).
const MinTTL = 1 * time.Minute

// MaxTTL is the maximum allowed TTL (30 days).
const MaxTTL = 30 * 24 * time.Hour

// DefaultKeyPrefix is the default prefix for Redis keys.
const DefaultKeyPrefix = "idempotency:"

// options contains Redis idempotency storage configuration.
type options struct {
	ttl       time.Duration `optgen:"default=DefaultTTL"`
	keyPrefix string        `optgen:"default=DefaultKeyPrefix"`
}
