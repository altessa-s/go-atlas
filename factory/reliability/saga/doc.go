// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory assembles a [saga.Orchestrator] from a [sagaconfig.Config] and
// injected backend clients.
//
// The builder is generic over the saga's shared data type T. It selects the
// state-store backend from sagaconfig.Config.Storage.Type. It builds only the
// memory store and, with [Builder.UseSQLDB], the sqldb one; the nats, mongo and
// redis stores register from reliability/saga/storages/<backend>/factory with
// [Builder.UseStorage], and a configured type without one fails at Build
// naming the package to add. Orchestrator tunables (timeouts, retries, recovery
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
//		UseStorage(sagaconfig.StorageTypeRedis, redisfactory.Builder(rdb)).
//		UseScheduler(scheduler).
//		Build(ctx)
//
// [Builder.UseStorage] accepts custom backend names. For standalone schema
// validation, pass [Builder.RegisteredStorageTypes] results to SetRegisteredTypes.
package factory
