// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides configuration-based creation of NATS connections
// and JetStream consumer configurations.
//
// It integrates with [config.Nats] and [config.NatsConsumer] to create NATS
// connections with NKey, token, or username/password authentication, optional
// TLS, automatic reconnection with unlimited buffer, and compression.
//
// JetStream consumer configurations are produced by
// [Factory.ConsumerConfigFromConfig], which maps [config.NatsConsumer] fields
// to [jetstream.ConsumerConfig] including delivery policy, ack policy, replay
// policy, backoff schedules, and performance tuning.
//
// When a [health.Coordinator] is provided via [WithHealthCoordinator], the
// factory registers a health checker that reports the connection status.
//
// Example:
//
//	f := factory.New(factory.WithLogger(logger))
//	conn, err := f.CreateConnectionFromConfig(cfg.Nats)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer conn.Close()
//
//	js, err := f.CreateJetStream(conn)
//	if err != nil {
//	    log.Fatal(err)
//	}
package factory
