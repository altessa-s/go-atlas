// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory assembles a [saga.Orchestrator] from a [config.Saga] and
// injected backend clients.
//
// The builder is generic over the saga's shared data type T. It selects the
// state-store backend from config.Saga.Storage.Type (memory, nats, mongo, or
// redis); the matching client must be injected via the fluent Use* methods, or
// Build returns an error. Orchestrator tunables (timeouts, retries, recovery
// schedule) are translated from the config; optional dependencies (collector,
// scheduler, leader elector, serializer, dead-letter hook) are injected.
//
// # Usage
//
//	cfg := config.DefaultSaga()
//	cfg.Storage = &config.SagaStorageConfig{
//		Type: config.SagaStorageTypeRedis,
//		Redis: &config.SagaRedisStorageConfig{KeysPrefix: "saga:"},
//	}
//
//	orch, err := factory.New(&cfg, def).
//		UseRedisClient(rdb).
//		UseScheduler(scheduler).
//		Build()
package factory
