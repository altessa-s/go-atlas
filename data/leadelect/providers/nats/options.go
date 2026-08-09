// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import (
	"log/slog"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas/data/internal/natskvlease"
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// DefaultBucket is the default KeyValue bucket name.
const DefaultBucket = "leadelect"

// DefaultBucketKeysTTL is the default TTL for keys (10 seconds).
const DefaultBucketKeysTTL = time.Second * 10

// DefaultRenewRatio is the fraction of the effective lease lifetime at which
// the lease is renewed, so renewals tick every
// min(electionTTL, DefaultBucketKeysTTL) × DefaultRenewRatio.
//
// It is the shared lease default; see [natskvlease.DefaultRenewRatio] for why
// the ratio is one third rather than something larger. What it buys here is a
// failure budget for holding leadership: at the former 0.75 a single dropped
// packet cost an election, because the next tick landed after the key had
// already expired server-side. Renewal traffic is one KV update per tick per
// elector, so the extra cost is negligible next to a spurious re-election.
const DefaultRenewRatio = natskvlease.DefaultRenewRatio

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
