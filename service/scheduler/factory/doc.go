// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory builds [scheduler.Scheduler] instances and their
// [scheduler.Storage] backends from [config.Scheduler] and
// [config.SchedulerStorageConfig] configuration objects.
//
// [Factory] holds optional infrastructure references -- a MongoDB database, a
// Redis client, and a leader elector -- that are injected once at construction
// via functional [Option] values and reused across every scheduler or storage
// backend the factory creates.
//
// Supported storage backends:
//
//   - In-memory (default) -- no external dependencies; suitable for development
//     and testing.
//   - MongoDB -- requires a [mongo.Database] supplied via [WithMongoDb].
//   - Redis -- requires a [redis.UniversalClient] supplied via [WithRedisClient].
//
// # Usage
//
//	f := factory.New(
//		factory.WithLogger(logger),
//		factory.WithLeaderElector(le),
//		factory.WithMongoDb(db),
//	)
//	storage, err := f.CreateStorageFromConfig(cfg.Storage)
//	if err != nil { ... }
//	sched, err := f.CreateSchedulerFromConfig(cfg, storage)
//	if err != nil { ... }
package factory
