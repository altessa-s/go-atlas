// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides a fluent builder for creating budget limiters
// from configuration.
//
// [BudgetLimiterBuilder] uses a fluent API with deferred error accumulation:
// errors from any step are collected and returned at [BudgetLimiterBuilder.Build] time.
//
//	limiter, err := factory.New(cfg.BudgetLimiter).
//	    UseLogger(logger).
//	    UseRedisClient(redisClient).
//	    Build()
package factory
