// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides configuration-based creation of generic outbox instances.
//
// The factory creates transport-agnostic outboxes backed by MongoDB. The caller provides
// a Handler function that determines how events are dispatched.
//
// Example usage:
//
//	f := factory.New(
//	    factory.WithLogger(logger),
//	    factory.WithScheduler(scheduler),
//	)
//
//	handler := func(ctx context.Context, event outbox.Event) error {
//	    return sendWebhook(ctx, event.Topic, event.Payload)
//	}
//
//	ob, err := f.CreateOutboxWithMongoFromConfig(cfg, db, handler)
//	if err != nil {
//	    log.Fatal(err)
//	}
package factory
