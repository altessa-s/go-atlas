// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides configuration-based creation of message brokers
// from [config] structures.
//
// # Components
//
// The [Factory] creates and configures the following components:
//
//   - [broker.Broker] via [Factory.CreateBrokerFromConfig]
//   - [natsprovider.Nats] via [Factory.CreateNatsProviderWithRecoveryFromConfig]
//   - [inprogress.Manager] via [Factory.CreateInProgressManagerFromConfig]
//   - [outbox.Outbox] via [Factory.CreateOutboxWithMongoFromConfig]
//   - [recovery.Manager] via [Factory.CreateRecoveryManagerFromConfig]
//
// # Scheduler Integration
//
// When configured with a scheduler (via [WithScheduler]), the factory
// automatically registers background tasks for outbox, recovery, and
// inprogress operations.
//
// # Example
//
//	f := factory.New(
//	    factory.WithLogger(logger),
//	    factory.WithScheduler(scheduler),
//	    factory.WithPublishConverter(publishConverter),
//	)
//
//	result, err := f.CreateNatsProviderWithRecoveryFromConfig(&cfg.Nats, natsConn)
//	if err != nil {
//	    return err
//	}
//	defer result.Close()
//
//	if result.Recovery != nil {
//	    result.Recovery.Start()
//	}
//
//	outbox, err := f.CreateOutboxWithMongoFromConfig(&cfg.Broker.Outbox, mongoDB, result.Provider)
//	broker, err := f.CreateBrokerFromConfig(&cfg.Broker, result.Provider)
package factory
