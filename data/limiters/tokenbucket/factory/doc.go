// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides a fluent builder for creating rate limiters
// from configuration.
//
// [TokenBucketLimiterBuilder] uses a fluent API with deferred error accumulation:
// errors from any step are collected and returned at [TokenBucketLimiterBuilder.Build] time.
//
//	limiter, err := factory.New(cfg.TokenBucketLimiter).
//	    UseLogger(logger).
//	    UseRedisClient(redisClient).
//	    Build()
package factory
