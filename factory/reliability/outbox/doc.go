// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides a fluent builder for creating generic outbox instances
// from configuration.
//
// [OutboxBuilder] uses a fluent API with deferred error accumulation:
// errors from any step are collected and returned at build time.
//
//	handler := func(ctx context.Context, event outbox.Event) error {
//	    return sendWebhook(ctx, event.Key, event.Payload)
//	}
//
//	cfg := outboxconfig.Default()
//	ob, err := factory.New(&cfg).
//	    UseLogger(logger).
//	    UseScheduler(scheduler).
//	    UseStorage(outboxconfig.StorageTypeMongo, outboxmongofactory.Builder(db)).
//	    UseSQLDB(sqlDB).
//	    Build(ctx, handler)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// The storage is selected by the config's storage.type. The builder builds the
// sqldb store over [OutboxBuilder.UseSQLDB]; the MongoDB storage comes from
// factory/reliability/outbox/storages/mongo through [OutboxBuilder.UseStorage],
// so this package does not import the MongoDB driver.
//
// A successful build with changeStream or notify starts the watcher. The
// shutdown hook or Outbox.StopWatch stops it and joins the worker. Both modes
// require a dispatch schedule and [OutboxBuilder.UseScheduler]. If registration
// fails, retry RegisterTasks on the returned outbox, then call StartWatch with
// the desired lifetime and arrange to call StopWatch. For notify, supply the
// PostgreSQL listener through [OutboxBuilder.UseSQLListener]. Its channel defaults
// to the table-derived name and can be overridden with storage.sqldb.notifyChannel.
//
// [OutboxBuilder.UseStorage] accepts custom backend names. For standalone schema
// validation, pass [OutboxBuilder.RegisteredStorageTypes] results to SetRegisteredTypes.
package factory
