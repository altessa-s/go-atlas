// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory maps the redis block of the storage backend configuration
// to a Redis cursor storage for factory/store/mongo.
//
// The cursor storage factory builds only the in-memory storage; register this
// one for the redis type:
//
//	storage, err := mongofactory.New(cfg.CursorStorage).
//		UseStorage(storageconfig.BackendTypeRedis, redisfactory.Builder(redisClient)).
//		WithTTL(24 * time.Hour).
//		Build(ctx)
package factory
