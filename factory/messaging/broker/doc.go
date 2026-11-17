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
//   - [broker.Provider] via [BrokerBuilder.CreateProvider]
//   - [inprogress.Manager] via [BrokerBuilder.CreateInProgressManager]
//   - [outbox.Outbox] via [BrokerBuilder.CreateOutbox], over the storage named
//     by outbox.storage.type
//
// The builder builds no broker provider itself: the NATS one registers with
// [BrokerBuilder.UseProvider] from factory/messaging/broker/providers/nats,
// which also builds the NATS stream recovery manager, and a configured
// provider without one fails naming the package to add. The MongoDB outbox
// storage registers with [BrokerBuilder.UseOutboxStorage] from
// factory/reliability/outbox/storages/mongo; the SQL one is built over
// [BrokerBuilder.UseSQLDB].
//
// # Scheduler Integration
//
// When configured with a scheduler (via [BrokerBuilder.UseScheduler]), the builder
// automatically registers background tasks for outbox and inprogress
// operations.
//
// # Example
//
//	b := factory.New(&cfg.Broker).
//	    UseLogger(logger).
//	    UseScheduler(scheduler).
//	    UsePublishConverter(publishConverter).
//	    UseProvider(brokerconfig.ProviderNATS, natsfactory.Builder(natsConn)).
//	    UseOutboxStorage(brokerconfig.OutboxStorageTypeMongo, outboxmongofactory.Builder(mongoDB))
//
//	provider, err := b.CreateProvider()
//	if err != nil {
//	    return err
//	}
//
//	outbox, err := b.CreateOutbox(ctx, provider)
//	brk, err := b.Build(provider)
package factory
