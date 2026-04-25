// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

import (
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas/core/runtime/appinfo"
)

// DefaultBucketKeysTTL is the default TTL for the bucket itself.
const DefaultBucketKeysTTL = time.Second * 10

// DefaultRenewRatio is the default ratio of TTL to use for renewing the lock.
const DefaultRenewRatio = 0.75

// DefaultOperationsTimeout is the default timeout for lock operations.
const DefaultOperationsTimeout = time.Second * 5

// options contains NATS locker configuration.
type options struct {
	logger     *slog.Logger
	bucket     string        `optgen:"default=defaultBucket()"`
	renewRatio float64       `optgen:"default=DefaultRenewRatio"`
	ttl        time.Duration `optgen:"default=DefaultBucketKeysTTL"`
}

func defaultBucket() string {
	return appinfo.Name + "-dlock"
}
