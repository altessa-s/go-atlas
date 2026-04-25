// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package nats provides NATS JetStream KeyValue cursor storage for MongoDB pagination.
// Enables distributed cursor sharing across application instances with automatic expiration.
//
// Example:
//
//	kv, _ := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
//	    Bucket: "cursors",
//	    TTL:    time.Hour,
//	})
//	storage := nats.New(kv)
//	result, _ := mongo.ListCursor(ctx, coll, mongo.WithListCursorStorage(storage))
package nats
