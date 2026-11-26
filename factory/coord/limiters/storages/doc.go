// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory declares the storage function shared by the limiter
// factories, factory/coord/limiters/budget and factory/coord/limiters/tokenbucket.
//
// The limiter factories build only the in-memory storage; a driver storage
// registers its [StorageFunc] from its own factory package, so one builder
// serves both limiters:
//
//	limiter, err := budgetfactory.New(cfg.BudgetLimiter).
//		UseStorage(limiterconfig.StorageTypeRedis, redisfactory.Builder(redisClient)).
//		Build(ctx)
package factory
