// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import (
	"log/slog"
	"time"
)

// DefaultCollection is the default collection holding one document per lock key.
const DefaultCollection = "dlocks"

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

// options contains MongoDB locker configuration.
type options struct {
	logger *slog.Logger `optgen:"default=slog.New(slog.DiscardHandler),notnil"`
	// collection holds the lock documents.
	collection string `optgen:"default=DefaultCollection"`
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
	// expiredCleanup enables the TTL index on expires_at (see
	// [WithExpiredCleanup]); expiredCleanupGrace is how long a document
	// outlives its lease.
	expiredCleanup      bool          `opt:"-"`
	expiredCleanupGrace time.Duration `opt:"-"`
}

// WithExpiredCleanup lets MongoDB delete a lock document grace after its
// lease expired or was released, through a TTL index on expires_at that
// [Locker.EnsureIndexes] creates. Without it the collection keeps one
// document per key ever locked. A deleted key starts again at fencing token 1,
// so fencing tokens are monotonic only while a key is locked again within
// grace of its last lease; choose grace longer than any holder may act on a
// token. MongoDB's TTL monitor runs about once a minute, so a document may
// outlive grace by that much; grace is rounded up to whole seconds. A
// negative grace makes [New] fail.
func WithExpiredCleanup(grace time.Duration) Option {
	return func(o *options) {
		o.expiredCleanup = true
		o.expiredCleanupGrace = grace
	}
}
