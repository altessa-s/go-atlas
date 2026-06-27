// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import (
	"log/slog"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas/observability/metrics"
)

// DefaultBucket is the default KeyValue bucket name.
const DefaultBucket = "leadelect"

// DefaultBucketKeysTTL is the default TTL for keys (10 seconds).
const DefaultBucketKeysTTL = time.Second * 10

// DefaultRenewRatio is the default lease renewal ratio (0.75).
const DefaultRenewRatio = 0.75

// DefaultStorage is the default JetStream storage backend for the election
// bucket. Memory storage keeps the lease ephemeral — it is intentionally lost
// when the server restarts, forcing a clean re-election — and avoids disk I/O on
// the renew hot path. Switch to [jetstream.FileStorage] via [WithStorage] when
// the bucket must survive a JetStream restart (e.g. to preserve the monotonic
// fencing revision across a full server bounce).
const DefaultStorage = jetstream.MemoryStorage

// options contains NATS provider configuration.
type options struct {
	logger     *slog.Logger
	bucket     string                `optgen:"default=DefaultBucket"`
	renewRatio float64               `optgen:"default=DefaultRenewRatio"`
	storage    jetstream.StorageType `optgen:"default=DefaultStorage"`
	collector  metrics.Collector     `optgen:"notnil"`
}
