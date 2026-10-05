// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides a fluent builder for creating OIDC providers
// from configuration.
//
// [ProviderBuilder] uses a fluent API with deferred error accumulation:
// errors from any step are collected and returned at [ProviderBuilder.Build] time.
//
//	provider, err := factory.New(cfg.OIDC).
//	    UseLogger(logger).
//	    UseTokenCache(tokenCache).
//	    UseRedisClient(redisClient).
//	    Build(ctx)
//
// The provider owns the revocation filter's rebuilds: it syncs the filter
// from revocation.source once at construction and then on
// revocation.syncSchedule. The filter is built without probfilter rebuild
// scheduling, and a revocation.filter.bloom.rebuildCron or
// rebuildOnStart: true is rejected by Build as a conflicting second schedule.
package factory
