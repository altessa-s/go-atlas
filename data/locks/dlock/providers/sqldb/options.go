// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import (
	"log/slog"
	"time"
)

// DefaultTableName is the default table holding one row per lock key; it
// matches the MongoDB provider's default collection name.
const DefaultTableName = "dlocks"

// DefaultTTL is the default lease of a lock: a holder that stops renewing
// loses the lock this long after its last renewal.
const DefaultTTL = 10 * time.Second

// DefaultRenewRatio is the fraction of the TTL after which the lease is
// renewed. One third leaves two more attempts inside one TTL, so a single
// failed round trip does not cost the lock.
const DefaultRenewRatio = 1.0 / 3.0

// DefaultOperationsTimeout bounds one acquisition attempt, one renewal and
// one release.
const DefaultOperationsTimeout = 5 * time.Second

// options contains SQL locker configuration.
type options struct {
	logger *slog.Logger `optgen:"default=slog.New(slog.DiscardHandler),notnil"`
	// tableName holds the lock rows, optionally schema-qualified.
	tableName string `optval:"nonempty" optgen:"default=DefaultTableName"`
	// ttl is the lease of a lock.
	ttl time.Duration `optgen:"default=DefaultTTL" optval:"positive"`
	// renewRatio is the fraction of ttl after which the lease is renewed;
	// it must lie in (0, 1).
	renewRatio float64 `optgen:"default=DefaultRenewRatio"`
	// operationsTimeout bounds one acquisition attempt, renewal or release.
	operationsTimeout time.Duration `optgen:"default=DefaultOperationsTimeout" optval:"positive"`
}
