// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory maps the redis block of the idempotency storage configuration
// to a Redis idempotency storage for factory/reliability/idempotency.
//
// The idempotency factory builds only the in-memory storage; register this one
// for the redis type:
//
//	keeper, err := idempotencyfactory.New(cfg.Idempotency).
//		UseStorage(idempotencyconfig.StorageTypeRedis, redisfactory.Builder(redisClient)).
//		Build()
package factory
