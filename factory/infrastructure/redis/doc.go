// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides a fluent builder for creating Redis clients
// from configuration.
//
// [ClientBuilder] uses a fluent API with deferred error accumulation:
// errors from any step are collected and returned at [ClientBuilder.Build] time.
//
//	owner, err := factory.New(cfg.Redis).
//	    UseLogger(logger).
//	    UseHealthCoordinator(coordinator).
//	    Build(ctx)
//
// After a successful build, pass owner.Client() to storages as a borrowed
// reference. Only the owner closes the driver and releases the health checker,
// either directly or through [ClientBuilder.UseShutdownHooks].
//
// The builder integrates with [redisconfig.Config] to create [redis.UniversalClient]
// instances with authentication, connection pooling, and support for
// standalone, sentinel, and cluster modes. The mode is determined
// automatically: sentinel when [redisconfig.Config.MasterName] is set, cluster
// when multiple hosts are provided, standalone otherwise.
//
// The builder supports two configuration paths: connection-URI based
// (when [redisconfig.Config.ConnectionURI] is set) and field-based (individual
// host, credential, and pool settings). Both paths converge in
// [ClientBuilder.UniversalOptions]. URI parse failures match
// [redisconfig.ErrInvalidConnectionURI].
//
// When a [health.ScopedReporter] is provided via [ClientBuilder.UseHealthCoordinator],
// the builder registers a health checker that pings Redis on each check.
//
// [ClientBuilder.Build] pings Redis before returning the client, with
// [DefaultPingTimeout] as an upper bound on the ping context. On failure or
// cancellation, the builder closes the client and includes any close error.
// For a service that must start while Redis is down, [ClientBuilder.SkipStartupPing]
// drops the ping and [ClientBuilder.WithoutHealth] drops the health checker;
// go-redis reconnects by itself once Redis is reachable.
//
// A service can install [NewSlogAdapter] with redis.SetLogger during startup
// to route go-redis diagnostics, including Sentinel discovery, to its logger.
package factory
