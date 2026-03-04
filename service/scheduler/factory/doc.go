// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides a fluent builder for creating [scheduler.Scheduler]
// instances and their storage backends from configuration.
//
// [SchedulerBuilder] uses a fluent API with deferred error accumulation:
// errors from any step are collected and returned at [SchedulerBuilder.Build] time.
//
// Supported storage backends:
//
//   - In-memory (default) -- no external dependencies; suitable for development
//     and testing.
//   - MongoDB -- requires a [mongo.Database] supplied via [SchedulerBuilder.UseMongoDb].
//   - Redis -- requires a [redis.UniversalClient] supplied via [SchedulerBuilder.UseRedisClient].
//
// # Usage
//
//	sched, err := factory.New(cfg).
//	    UseLogger(logger).
//	    UseLeaderElector(le).
//	    UseMongoDb(db).
//	    Build()
//	if err != nil { ... }
package factory
