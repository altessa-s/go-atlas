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
//	cache, err := factory.NewBuilder("denylist", cfg.Filter, &defaults, redisDenylist).
//	    UseLogger(logger).
//	    UseRedisClient(redisClient). // only for a Redis-backed negative filter
//	    UseMetrics(collector, "auth_denylist_negcache").
//	    Build()
package factory
