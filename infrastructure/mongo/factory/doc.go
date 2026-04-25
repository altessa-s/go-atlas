// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides a fluent builder for creating MongoDB clients
// from configuration.
//
// [MongoBuilder] uses a fluent API with deferred error accumulation:
// errors from any step are collected and returned at [MongoBuilder.Build] time.
//
//	m, err := factory.New(cfg.Mongodb).
//	    UseLogger(logger).
//	    UseTlsConfig(tlsConfig).
//	    UseKmsProvider(kmsProvider).
//	    UseHealthCoordinator(coordinator).
//	    Build(ctx)
//
// The builder integrates with [config.Mongodb] to produce driver-level
// [go.mongodb.org/mongo-driver/v2/mongo/options.ClientOptions] and
// higher-level [mongo.Mongo] wrappers with authentication, TLS, connection
// pooling, compression, and client-side field level encryption (CSFLE).
//
// The builder supports two configuration modes: connection-URI based
// (when [config.Mongodb.ConnectionURI] is set) and field-based (individual
// host, credential, and pool settings). Both paths converge in
// [MongoBuilder.ClientOptions].
//
// When a [health.Coordinator] is provided via [MongoBuilder.UseHealthCoordinator],
// the builder automatically registers a health checker that pings the
// primary on each check.
package factory
