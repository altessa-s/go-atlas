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

// DefaultRenewRatio is the fraction of the effective lease lifetime at which
// the lease is renewed, so renewals tick every
// min(electionTTL, DefaultBucketKeysTTL) × DefaultRenewRatio.
//
// The ratio decides how many renewal attempts fall inside one lease lifetime —
// floor(1/ratio) of them — which is the failure budget for holding leadership:
//
//	0.75 → 1 attempt.  A single dropped packet or slow round trip costs
//	                   leadership, because the next tick lands after the key
//	                   has already expired server-side.
//	1/3  → 3 ticks inside the window, two of them with room to spare. One
//	       transient failure is absorbed without a spurious re-election.
//
// One third is the usual choice for lease keepalives (etcd sessions and
// ZooKeeper heartbeats both renew at TTL/3; Kubernetes' leader election retries
// more often still). It also keeps each attempt clear of the next: a renewal
// exhausting its internal retry budget takes at most a couple of seconds, well
// under the ~3.3s tick at the default TTL.
//
// Renewal traffic is one KV update per tick per elector, so the extra cost over
// a larger ratio is negligible next to losing leadership to a single blip.
const DefaultRenewRatio = 1.0 / 3.0

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
