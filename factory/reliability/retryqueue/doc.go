// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory assembles a [retryqueue.Storage] from a
// [retryqueueconfig.Config] and injected backend clients.
//
// The builder is generic over the payload type P. It selects the backend from
// retryqueueconfig.Config.Storage.Type. It builds only the memory store and,
// with [Builder.UseSQLDB], the sqldb one; the mongo store registers from
// factory/reliability/retryqueue/storages/mongo with [Builder.UseStorage], and
// a configured mongo type without one fails at Build naming the package to
// add. Every bundled store also implements [retryqueue.Mover].
//
// # Usage
//
//	cfg := retryqueueconfig.Default()
//	cfg.Storage = &retryqueueconfig.StorageConfig{
//		Type: retryqueueconfig.StorageTypeSQL,
//		SQL:  &retryqueueconfig.SQLStorageConfig{Dialect: "postgres", Table: "retry_items", EnsureSchema: true},
//	}
//
//	queue, err := factory.New[Valuation](&cfg).
//		UseSQLDB(db).
//		Build(ctx)
//
// # Purging
//
// SQL configuration maps purgeInterval and purgeBatchSize to the store options (defaults "1m" and
// 1000). A zero interval disables background purging. The builder registers StopPurge with
// UseShutdownHooks(group) or the process-wide runtime shutdown registry; run shutdown hooks before
// closing the injected database.
//
// [Builder.UseStorage] accepts custom backend names. For standalone schema
// validation, pass [Builder.RegisteredStorageTypes] results to SetRegisteredTypes.
package factory
