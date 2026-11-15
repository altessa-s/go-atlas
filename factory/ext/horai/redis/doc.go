// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory maps the redis block of the scheduler storage
// configuration to a Redis task storage for factory/ext/horai,
// creating the RediSearch indexes at startup when ensureIndexes is set.
//
// The scheduler factory builds only the memory and sqldb task storages;
// register this one for the redis type:
//
//	sched, err := schedulerfactory.New(&cfg.Scheduler).
//		UseStorage(schedulerconfig.StorageTypeRedis, redisfactory.Builder(redisClient)).
//		Build(ctx)
package factory
