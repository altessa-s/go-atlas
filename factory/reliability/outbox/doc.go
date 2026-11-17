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
//	    return sendWebhook(ctx, event.Topic, event.Payload)
//	}
//
//	ob, err := factory.New(cfg).
//	    UseLogger(logger).
//	    UseScheduler(scheduler).
//	    UseStorage(brokerconfig.OutboxStorageTypeMongo, outboxmongofactory.Builder(db)).
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
// With watch set to changeStream or notify, [OutboxBuilder.Build] starts
// outbox.Outbox.Watch and stops it through the shutdown hooks; notify needs the
// PostgreSQL listener from [OutboxBuilder.UseSQLListener]. Both push modes
// require a dispatch schedule and [OutboxBuilder.UseScheduler]. A task
// registration failure returns a non-nil outbox with its watcher already
// started; retain that instance to retry RegisterTasks. Shutdown hooks still
// stop the watcher. The notify channel defaults to the table-derived name;
// storage.sqldb.notifyChannel overrides it.
package factory
