// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory assembles a [saga.Orchestrator] from a [sagaconfig.Config] and
// injected backend clients.
//
// The builder is generic over the saga's shared data type T. It selects the
// state-store backend from sagaconfig.Config.Storage.Type (memory, nats, mongo,
// redis, or sqldb); the matching client must be injected via the fluent Use* methods, or
// Build returns an error. Orchestrator tunables (timeouts, retries, recovery
// schedule) are translated from the config; optional dependencies (collector,
// scheduler, leader elector, serializer, dead-letter hook) are injected.
//
// # Usage
//
//	cfg := sagaconfig.Default()
//	cfg.Storage = &sagaconfig.StorageConfig{
//		Type: sagaconfig.StorageTypeRedis,
//		Redis: &sagaconfig.RedisStorageConfig{KeysPrefix: "saga:"},
//	}
//
//	orch, err := factory.New(&cfg, def).
//		UseRedisClient(rdb).
//		UseScheduler(scheduler).
//		Build()
package factory
