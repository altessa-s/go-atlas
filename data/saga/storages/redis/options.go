// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import (
	"time"
)

// DefaultKeyPrefix is the default prefix applied to all Redis keys.
const DefaultKeyPrefix = "saga:"

// options contains Redis saga storage configuration.
type options struct {
	keyPrefix string        `optgen:"default=DefaultKeyPrefix"`
	ttl       time.Duration // 0 → persist (no expiry); >0 → PEXPIRE applied on every write
}
