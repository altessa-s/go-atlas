// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory maps the redis block of the limiter storage configuration
// to a Redis limiter storage for factory/coord/limiters/budget and
// factory/coord/limiters/tokenbucket.
//
// The limiter factories build only the in-memory storage; register this one
// for the redis type:
//
//	limiter, err := tokenbucketfactory.New(cfg.TokenBucketLimiter).
//		UseStorage(limiterconfig.StorageTypeRedis, redisfactory.Builder(redisClient)).
//		Build(ctx)
package factory
