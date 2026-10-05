// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides a fluent builder that constructs a
// [github.com/altessa-s/go-atlas/auth/denylist/negcache.Cache] from a
// probabilistic-filter configuration and injected dependencies.
//
// [Builder] builds the negative filter through the shared probfilter factory
// and wires it to a caller-supplied [negcache.Authoritative] revocation store.
// Following the project's inject-deps-at-the-consumer convention, the factory
// never constructs the authoritative store (a Redis-backed denylist, a database,
// etc.) or a Redis client — the caller injects those and the factory only
// assembles the cache around them.
//
// Errors from any fluent step, plus missing-dependency and filter-construction
// errors, are reported at [Builder.Build] time.
//
// # Rebuilds
//
// The probfilter factory owns the negative filter's rebuilds. Its loader is
// the one set with [Builder.UseDataLoader], else the authoritative store
// itself when it implements probfilter.DataLoader (as
// auth/denylist/storages/redis.Store does). With a loader, the Bloom
// settings apply: rebuildOnStart rebuilds inside Build (the cache is
// returned populated; a failed rebuild fails Build), and rebuildCron
// rebuilds an in-memory filter on a process-local cron and a Redis filter
// through the scheduler set with [Builder.UseScheduler]. When another
// process is rebuilding a shared Redis filter during Build, Build still
// succeeds: the cache defers to the authoritative store until a rebuild of
// the shared filter is committed. Without a loader the settings are inert;
// a Cuckoo filter has no rebuild settings — rebuild it through
// negcache.Cache.Rebuild. Close the cache to stop its rebuilds.
//
//	cache, err := factory.NewBuilder("denylist", cfg.Filter, &defaults, redisDenylist).
//	    UseLogger(logger).
//	    UseRedisClient(redisClient). // only for a Redis-backed negative filter
//	    UseScheduler(scheduler).     // runs rebuildCron of a Redis-backed filter
//	    UseMetrics(collector, "auth_denylist_negcache").
//	    Build()
//	defer cache.Close(ctx)
package factory
