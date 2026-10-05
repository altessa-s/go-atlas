// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import (
	"log/slog"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas/core/runtime/appinfo"
	"github.com/altessa-s/go-atlas/data/internal/natskvlease"
)

// DefaultBucketKeysTTL is the default lock TTL, and with it the key TTL of the
// bucket the locks live in — the two are the same duration by construction, so
// a lock the holder stops renewing is reaped exactly when its lease says.
const DefaultBucketKeysTTL = time.Second * 10

// DefaultRenewRatio is the fraction of the lock TTL at which the lease is
// renewed. See [natskvlease.DefaultRenewRatio] for why it is one third: the
// ratio sets how many renewal attempts fall inside one TTL, and at 0.75 there
// was exactly one, so a single dropped round trip cost the lock.
const DefaultRenewRatio = natskvlease.DefaultRenewRatio

// DefaultStorage is the default storage of the lock bucket. Memory: a lock
// lives no longer than its TTL, so nothing in the bucket needs to survive a
// server restart.
const DefaultStorage = jetstream.MemoryStorage

// DefaultOperationsTimeout is the default timeout for lock operations.
const DefaultOperationsTimeout = time.Second * 5

// options contains NATS locker configuration.
type options struct {
	logger     *slog.Logger
	bucket     string        `optgen:"default=defaultBucket()"`
	renewRatio float64       `optgen:"default=DefaultRenewRatio"`
	ttl        time.Duration `optgen:"default=DefaultBucketKeysTTL"`
	// storage is the storage type of a bucket New creates and the target of
	// MigrateBucketStorage. An existing bucket keeps its storage type.
	storage jetstream.StorageType `optgen:"default=DefaultStorage"`
	// acquireTimeout bounds the single acquisition attempt. It is deliberately
	// separate from the context passed to Lock: that context scopes the lock's
	// lifetime, and folding an acquisition deadline into it would release the
	// lock the moment the deadline passed — mid critical section.
	acquireTimeout time.Duration `optgen:"default=DefaultOperationsTimeout"`

	// migrateBucketTTL updates a pre-existing bucket whose key TTL differs
	// from the configured one instead of rejecting it with
	// [ErrBucketTTLMismatch]. Off by default: the bucket's TTL governs every
	// key in it, including keys of other processes sharing the bucket.
	migrateBucketTTL bool

	// strictBucketStorage rejects a pre-existing bucket whose storage type
	// differs from the one this backend asks for with
	// [ErrBucketStorageMismatch] instead of adopting it with a warning.
	strictBucketStorage bool
}

func defaultBucket() string {
	return appinfo.Name + "-dlock"
}
