// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory maps the broker configuration to a NATS JetStream broker
// provider and its stream recovery manager for factory/messaging/broker.
//
// The broker factory builds no provider itself; register this one for the
// nats provider type:
//
//	b := brokerfactory.New(&cfg.Broker).
//		UseProvider(brokerconfig.ProviderNATS, natsfactory.Builder(natsConn))
//	provider, err := b.CreateProvider()
//
// When the configuration enables stream recovery, build the provider together
// with its recovery manager instead and pass result.Provider to the broker
// builder:
//
//	result, err := natsfactory.NewWithRecovery(natsConn, &cfg.Broker,
//		brokerfactory.ProviderEnv{Logger: logger, Scheduler: scheduler})
//	if err != nil {
//		return err
//	}
//	defer result.Close()
//	brk, err := b.Build(result.Provider)
package factory
