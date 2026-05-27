// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides a fluent builder for creating Meilisearch clients
// from configuration.
//
// [ClientBuilder] uses a fluent API with deferred error accumulation:
// errors from any step are collected and returned at [ClientBuilder.Build] time.
//
//	client, err := factory.New(cfg.Meilisearch).
//	    UseLogger(logger).
//	    UseHealthCoordinator(coordinator).
//	    Build(ctx)
//
// The builder integrates with [config.Meilisearch] to create
// [meilisearch.Client] instances with the configured host, optional API key,
// and HTTP timeout. Build performs an initial health check against the
// server and returns an error if it is unreachable.
//
// When a [health.Coordinator] is provided via [ClientBuilder.UseHealthCoordinator],
// the builder registers a health checker that pings Meilisearch on each
// check under the service name "meilisearch" (overridable via
// [ClientBuilder.UseHealthServiceName]).
package factory
