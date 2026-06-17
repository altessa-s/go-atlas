// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package nats provides a durable [saga.Store] backed by a NATS JetStream
// KeyValue bucket. Each saga instance is a JSON document keyed by its ID; the
// bucket's monotonically increasing revision is used directly as the
// optimistic-concurrency token, so concurrent coordinators are serialized per
// instance without extra bookkeeping.
//
// Unlike the in-memory backend, state survives process restarts, so the
// orchestrator's recovery cycle can resume or roll back instances after a crash.
//
// # Lifecycle and retention
//
// Instances are removed explicitly when terminal (via Store.Delete). The bucket
// also carries a long backstop TTL (see [DefaultBucketTTL]); active sagas reset
// it on every checkpoint write, and stalled ones are recovered well inside the
// window. FetchRecoverable scans the whole bucket — adequate for moderate
// instance counts; very high-volume deployments should prefer a query-capable
// backend.
//
// # Usage
//
//	nc, _ := nats.Connect(url)
//	js, _ := jetstream.New(nc)
//	store, err := natsstore.New(js, natsstore.WithBucket("saga"))
//	orch := saga.New(store, def, saga.WithSagaTimeout(5*time.Minute))
package nats
