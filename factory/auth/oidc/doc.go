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
//	    UseRevocationBloomStorage(probfilterconfig.StorageTypeRedis, bloomredisfactory.Builder(redisClient)).
//	    Build(ctx)
//
// LoadServiceConfigOptions reads a JSON service policy and applies its settings
// after the caller options supplied for oidc.NewProvider. The JSON service
// policy and YAML OIDC config both map minimum and maximum token lifetimes
// into the provider's default and preset validation options.
//
// The revocation filter is built through factory/cache/probfilter, which
// builds only in-memory storages; a Redis filter storage registers with
// [ProviderBuilder.UseRevocationBloomStorage] or
// [ProviderBuilder.UseRevocationCuckooStorage] from
// factory/cache/probfilter/bloom/storages/redis or
// factory/cache/probfilter/cuckoo/storages/redis.
//
// The provider owns the revocation filter's rebuilds: it syncs the filter
// from revocation.source once at construction and then on
// revocation.syncSchedule. The filter is built without probfilter rebuild
// scheduling, and a revocation.filter.bloom.rebuildCron or
// rebuildOnStart: true is rejected by Build as a conflicting second schedule.
// revocation.maxStaleness maps to oidc.WithRevocationMaxStaleness,
// revocation.syncTimeout to oidc.WithRevocationSyncTimeout,
// revocation.allowEmpty to oidc.WithRevocationAllowEmpty, and
// revocation.maxResponseBytes to the URL loader's MaxResponseBytes.
//
// The context passed to Build bounds construction only: the caller owns the
// returned provider and must call its Close on shutdown. Build releases a filter
// it allocated on failure and transfers its cleanup to the provider on success.
package factory
