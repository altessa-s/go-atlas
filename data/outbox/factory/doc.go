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
//	    BuildWithMongoDB(db, handler)
//	if err != nil {
//	    log.Fatal(err)
//	}
package factory
