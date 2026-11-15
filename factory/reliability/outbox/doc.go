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
//	    BuildWithStorage(ctx, outboxmongofactory.Builder(db), handler)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// The builder builds only the SQL storage ([OutboxBuilder.BuildWithSQLDB]).
// The MongoDB storage comes from factory/reliability/outbox/storages/mongo through
// [OutboxBuilder.BuildWithStorage], so this package does not import the
// MongoDB driver.
package factory
