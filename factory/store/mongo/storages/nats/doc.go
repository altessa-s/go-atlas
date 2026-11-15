// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory maps the nats block of the storage backend configuration
// to a NATS JetStream KeyValue cursor storage for factory/store/mongo.
//
// The cursor storage factory builds only the in-memory storage; register this
// one for the nats type:
//
//	storage, err := mongofactory.New(cfg.CursorStorage).
//		UseStorage(storageconfig.BackendTypeNATS, natsfactory.Builder(js)).
//		WithTTL(24 * time.Hour).
//		Build(ctx)
//
// WithTTL is mandatory for this storage: the bucket's key TTL is the only
// thing that removes cursors, so a zero or negative TTL fails the build. A
// bucket that already exists without a TTL fails with [ErrBucketTTLMismatch]
// unless migrateBucketTTL is set, which updates it in place.
package factory
