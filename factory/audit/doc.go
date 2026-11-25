// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides a fluent builder for creating audit auditors
// from configuration.
//
// [AuditorBuilder] uses a fluent API with deferred error accumulation:
// errors from any step are collected and returned at [AuditorBuilder.Build] time.
//
//	auditor, err := factory.New(cfg.Audit).
//	    UseLogger(logger).
//	    UseDispatcher(eng).
//	    Build(ctx)
//
// The builder builds the memory and sqldb storages. The MongoDB and
// ClickHouse ones are registered with [AuditorBuilder.UseStorage] from
// factory/audit/storages/mongo and factory/audit/storages/clickhouse:
//
//	auditor, err := factory.New(cfg.Audit).
//	    UseStorage(auditconfig.StorageTypeMongo, auditmongofactory.Builder(db)).
//	    Build(ctx)
//
// SQL storage maps storage.sqldb.ttl to WithTTL and uses the builder's logger
// for retention failures. Zero keeps events forever; positive TTL starts
// background batched purges while open, unlike MongoDB's server TTL index.
// BuildStorage registers SQL storage shutdown; Build closes its storage after
// draining the owned dispatcher on shutdown and on failed engine startup.
//
// # Purging
//
// SQL configuration maps purgeInterval and purgeBatchSize to the store options (defaults "1m" and
// 1000). A zero interval disables background purging. Build registers shutdown through
// UseShutdownHooks(group) or the process-wide runtime registry and closes storage after draining
// dispatch. BuildStorage callers own the returned storage and must close it before the database.
//
// [AuditorBuilder.UseStorage] accepts custom backend names. For standalone schema
// validation, pass [AuditorBuilder.RegisteredStorageTypes] results to SetRegisteredTypes.
package factory
