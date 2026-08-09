// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import (
	"log/slog"
	"time"

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

// DefaultOperationsTimeout is the default timeout for lock operations.
const DefaultOperationsTimeout = time.Second * 5

// options contains NATS locker configuration.
type options struct {
	logger     *slog.Logger
	bucket     string        `optgen:"default=defaultBucket()"`
	renewRatio float64       `optgen:"default=DefaultRenewRatio"`
	ttl        time.Duration `optgen:"default=DefaultBucketKeysTTL"`
	// acquireTimeout bounds the single acquisition attempt. It is deliberately
	// separate from the context passed to Lock: that context scopes the lock's
	// lifetime, and folding an acquisition deadline into it would release the
	// lock the moment the deadline passed — mid critical section.
	acquireTimeout time.Duration `optgen:"default=DefaultOperationsTimeout"`
}

func defaultBucket() string {
	return appinfo.Name + "-dlock"
}
