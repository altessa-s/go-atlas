// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

//go:generate go tool optgen generate

import (
	"context"
	"database/sql"
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

// DefaultPurgeInterval is the interval between background purge passes.
const DefaultPurgeInterval = time.Minute

// DefaultPurgeBatchSize bounds the rows deleted by one purge statement.
const DefaultPurgeBatchSize = 1000

// options contains SQL locker configuration.
type options struct {
	// WithTxResolver finds the application transaction for a call.
	txResolver func(context.Context) (*sql.Tx, bool) `optgen:"notnil"`
	// WithPurgeInterval sets the interval between purge passes. Zero disables
	// the background loop; manual PurgeExpired calls remain available.
	purgeInterval time.Duration `optcheck:"min=0" optgen:"default=DefaultPurgeInterval,onfail=skip"`
	// WithPurgeBatchSize bounds the rows deleted by each statement. On MySQL it
	// must be at most 65534 (the placeholder limit of the key list), or [New]
	// fails. Nonpositive values leave the configured size unchanged.
	purgeBatchSize int          `optval:"positive" optgen:"default=DefaultPurgeBatchSize"`
	logger         *slog.Logger `optgen:"default=slog.New(slog.DiscardHandler),notnil"`
	// tableName holds the lock rows, optionally schema-qualified.
	tableName string `optval:"nonempty" optgen:"default=DefaultTableName"`
	// ttl is the lease of a lock.
	ttl time.Duration `optgen:"default=DefaultTTL" optval:"positive"`
	// renewRatio is the fraction of ttl after which the lease is renewed;
	// it must lie in (0, 1).
	renewRatio float64 `optgen:"default=DefaultRenewRatio"`
	// operationsTimeout bounds one acquisition attempt, renewal or release.
	operationsTimeout time.Duration `optgen:"default=DefaultOperationsTimeout" optval:"positive"`
	// withoutRenewal makes every lease a hard lease that is never renewed:
	// it ends one ttl after the acquisition request (or earlier, on release
	// or when the lock's context ends); renewRatio is then ignored.
	withoutRenewal bool `opt:"Renewal" optval:"invert"`
	// Enabled by generated WithExpiredCleanup.
	expiredCleanup bool `opt:"-"`
	// WithExpiredCleanup makes the locker delete a lock row grace after its
	// lease expired or was released, judged by the database clock. A
	// background sweep starts on the first Lock and stops at [Locker.Close];
	// [WithPurgeInterval] and [WithPurgeBatchSize] control its schedule and batches;
	// [Locker.PurgeExpired] runs one sweep on demand. Without it the table
	// keeps one row per key ever locked. A deleted key starts again at fencing
	// token 1, so fencing tokens are monotonic only while a key is locked again
	// within grace of its last lease; choose grace longer than any holder may
	// act on a token. A row may outlive grace by up to one sweep interval. A
	// negative grace makes [New] fail.
	expiredCleanupGrace time.Duration `opt:"ExpiredCleanup" optgen:"enables=expiredCleanup" optval:"nonpositive"`
}
