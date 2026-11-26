// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package nats provides a durable [saga.Storage] backed by a NATS JetStream
// KeyValue bucket. Each saga instance is a JSON document keyed by its ID; the
// bucket's monotonically increasing revision is used directly as the
// optimistic-concurrency token, so concurrent coordinators are serialized per
// instance without extra bookkeeping. An ID the store cannot represent yields an error matching saga.ErrInvalidStorageKey.
//
// Unlike the in-memory backend, state survives process restarts, so the
// orchestrator's recovery cycle can resume or roll back instances after a crash.
//
// # Lifecycle and retention
//
// Instances are removed explicitly when terminal (via Store.Delete). The bucket
// also carries a long backstop TTL (see [DefaultBucketTTL]); active sagas reset
// it on every checkpoint write, and stalled ones are recovered well inside the
// window.
//
// # Recoverable index
//
// Live instances are listed in a recoverable index in the same bucket, one
// "=rec.<key>" entry each ('=' before a non-hex digit never starts an instance
// key). The entry is written before the instance record on Create and on every
// non-terminal Update, and removed after the terminal write, so a crash between
// the two writes leaves at worst a dangling entry (dropped by the scan), never a
// live instance without an entry. FetchRecoverable reads only the index and the
// instances it lists, never the terminal backlog. DeleteTerminal has no index and
// walks the whole bucket, so run retention at a modest cadence.
//
// # Usage
//
//	nc, _ := nats.Connect(url)
//	js, _ := jetstream.New(nc)
//	store, err := natsstore.New(ctx, js, natsstore.WithBucket("saga"))
//	orch := saga.New(store, def, saga.WithSagaTimeout(5*time.Minute))
package nats
