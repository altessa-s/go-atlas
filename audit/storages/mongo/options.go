// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"math"
	"time"
)

//go:generate go tool optgen generate --type=options

const (
	// DefaultCollectionName is the default MongoDB collection for audit events.
	DefaultCollectionName = "audit_events"

	// DefaultIndexTimeout is the default timeout for index creation operations.
	DefaultIndexTimeout = 30 * time.Second

	// MinTTL is the smallest positive retention duration MongoDB's
	// expireAfterSeconds index option can represent without truncating to zero.
	MinTTL = time.Second

	// MaxTTL is the largest retention duration MongoDB's signed 32-bit
	// expireAfterSeconds index option can represent.
	MaxTTL = time.Duration(math.MaxInt32) * time.Second
)

type options struct {
	collectionName string        `optval:"nonempty" optgen:"default=DefaultCollectionName"`
	indexTimeout   time.Duration `optval:"positive" optgen:"default=DefaultIndexTimeout"`
	ttl            time.Duration `optval:"positive"`
}
