// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

import (
	"time"
)

// DefaultPrefix is the default prefix used for Redis keys.
const DefaultPrefix = "uniq:"

// DefaultTTL is the default Time To Live (TTL) for keys in Redis.
const DefaultTTL = time.Hour * 24

// options contains Redis uniq provider configuration.
type options struct {
	prefix string        `optgen:"default=DefaultPrefix"`
	ttl    time.Duration `optgen:"default=DefaultTTL"`
}
