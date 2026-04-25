// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides a fluent builder for creating message brokers
// and related components from configuration.
//
// [BrokerBuilder] uses a fluent API with deferred error accumulation:
// errors from any step are collected and returned at [BrokerBuilder.Build] time.
//
// # Components
//
// The builder creates and configures the following components:
//
//   - [broker.Broker] via [BrokerBuilder.Build]
//   - [natsprovider.Nats] via [BrokerBuilder.CreateNatsProviderWithRecovery]
//   - [inprogress.Manager] via [BrokerBuilder.CreateInProgressManager]
//   - [outbox.Outbox] via [BrokerBuilder.CreateOutboxWithMongoDB]
//   - [recovery.Manager] via [BrokerBuilder.CreateRecoveryManager]
//
// # Scheduler Integration
//
// When configured with a scheduler (via [BrokerBuilder.UseScheduler]), the builder
// automatically registers background tasks for outbox, recovery, and
// inprogress operations.
//
// # Example
//
//	b := factory.New(&cfg.Broker).
//	    UseLogger(logger).
//	    UseScheduler(scheduler).
//	    UsePublishConverter(publishConverter)
//
//	result, err := b.CreateNatsProviderWithRecovery(natsConn)
//	if err != nil {
//	    return err
//	}
//	defer result.Close()
//
//	if result.Recovery != nil {
//	    result.Recovery.Start()
//	}
//
//	outbox, err := b.CreateOutboxWithMongoDB(mongoDB, result.Provider)
//	brk, err := b.Build(result.Provider)
package factory
