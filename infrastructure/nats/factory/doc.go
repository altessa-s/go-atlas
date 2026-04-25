// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides a fluent builder for creating NATS connections
// and JetStream consumer configurations from configuration.
//
// [ConnectionBuilder] uses a fluent API with deferred error accumulation:
// errors from any step are collected and returned at [ConnectionBuilder.Build] time.
//
//	conn, err := factory.New(cfg.Nats).
//	    UseLogger(logger).
//	    UseTlsConfig(tlsConfig).
//	    UseHealthCoordinator(coordinator).
//	    Build()
//
// The builder integrates with [config.Nats] and [config.NatsConsumer] to create NATS
// connections with NKey, token, or username/password authentication, optional
// TLS, automatic reconnection with unlimited buffer, and compression.
//
// JetStream consumer configurations are produced by
// [ConsumerConfig], which maps [config.NatsConsumer] fields
// to [jetstream.ConsumerConfig] including delivery policy, ack policy, replay
// policy, backoff schedules, and performance tuning.
//
// When a [health.Coordinator] is provided via [ConnectionBuilder.UseHealthCoordinator],
// the builder registers a health checker that reports the connection status.
package factory
