// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory builds a github.com/altessa-s/go-horai [scheduler.Scheduler]
// and its storage backends from configuration, and reports the scheduler's
// observations as go-atlas metrics ([SchedulerBuilder.UseMeter]).
//
// [SchedulerBuilder] uses a fluent API with deferred error accumulation:
// errors from any step are collected and returned at [SchedulerBuilder.Build] time.
//
// [BuildLimitFunc] converts scheduler concurrency settings into a runtime limit function.
//
// Supported storage backends:
//
//   - In-memory (default) -- no external dependencies; suitable for development
//     and testing.
//   - MongoDB -- registered with [SchedulerBuilder.UseStorage] from
//     factory/ext/horai/mongo.
//   - Redis -- registered with [SchedulerBuilder.UseStorage] from
//     factory/ext/horai/redis.
//
// The historyStorage section moves execution history to a backend of its own:
//
//   - ClickHouse -- registered with [SchedulerBuilder.UseHistoryStorage] from
//     factory/ext/horai/clickhouse.
//
// # Usage
//
//	sched, err := factory.New(cfg).
//	    UseLogger(logger).
//	    UseLeaderElector(le).
//	    UseStorage(schedulerconfig.StorageTypeMongo, mongofactory.Builder(db)).
//	    Build(ctx)
//	if err != nil { ... }
//
// [SchedulerBuilder.UseStorage] accepts custom backend names. For standalone schema
// validation, pass [SchedulerBuilder.RegisteredStorageTypes] results to SetRegisteredTypes.
package factory
