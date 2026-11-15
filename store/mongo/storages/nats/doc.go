// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package nats provides NATS JetStream KeyValue cursor storage for MongoDB pagination.
// Enables distributed cursor sharing across application instances with automatic expiration.
//
// # Expiration
//
// Cursors are never deleted; the bucket's key TTL is their only cleanup. [New]
// therefore rejects a bucket without a positive key TTL, which would grow by
// one entry per issued page forever. [DefaultTTL] is the recommended value.
//
// Example:
//
//	kv, _ := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
//	    Bucket: "cursors",
//	    TTL:    nats.DefaultTTL,
//	})
//	storage, _ := nats.New(ctx, kv)
//	result, _ := mongo.ListCursor(ctx, coll, mongo.WithListCursorStorage(storage))
package nats
