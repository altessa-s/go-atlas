// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides a fluent builder for creating Redis clients
// from configuration.
//
// [ClientBuilder] uses a fluent API with deferred error accumulation:
// errors from any step are collected and returned at [ClientBuilder.Build] time.
//
//	client, err := factory.New(cfg.Redis).
//	    UseLogger(logger).
//	    UseHealthCoordinator(coordinator).
//	    Build(ctx)
//
// The builder integrates with [config.Redis] to create [redis.UniversalClient]
// instances with authentication, connection pooling, and support for
// standalone, sentinel, and cluster modes. The mode is determined
// automatically: sentinel when [config.Redis.MasterName] is set, cluster
// when multiple hosts are provided, standalone otherwise.
//
// The builder supports two configuration paths: connection-URI based
// (when [config.Redis.ConnectionURI] is set) and field-based (individual
// host, credential, and pool settings). Both paths converge in
// [ClientBuilder.UniversalOptions].
//
// When a [health.Coordinator] is provided via [ClientBuilder.UseHealthCoordinator],
// the builder registers a health checker that pings Redis on each check.
package factory
