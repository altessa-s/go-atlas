// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides a fluent builder for creating idempotency keepers
// from configuration.
//
// [KeeperBuilder] uses a fluent API with deferred error accumulation:
// errors from any step are collected and returned at [KeeperBuilder.Build] time.
//
// # Caller binding
//
// The Keeper binds every record to the verified principal of the request
// (reliability/idempotency/principalowner.PrincipalOwner): another caller presenting the key is refused
// the record. A request without a principal records no owner, and the
// transports then never replay the record or echo its entity ID. Override with
// [KeeperBuilder.UseOwner], [KeeperBuilder.UseKeyNamespace] and
// [KeeperBuilder.UseReplayAuthorizer].
//
// # Storages
//
// The builder builds memory and SQL storage (with [KeeperBuilder.UseSQLDB]).
// SQL cleanup starts lazily and is closed through UseShutdownHooks or the process
// shutdown registry; the caller closes the database after the hooks run.
// SQL schema creation is optional and bounded by a 30-second timeout. Redis, NATS and MongoDB
// ones register from reliability/idempotency/storages/<backend>/factory with
// [KeeperBuilder.UseStorage]; a configured type without one fails at Build
// naming the package to add.
//
// # Usage
//
//	keeper, err := factory.New(cfg.Idempotency).
//	    UseLogger(logger).
//	    UseStorage(idempotencyconfig.StorageTypeRedis, redisfactory.Builder(redisClient)).
//	    Build()
//
// # Purging
//
// SQL configuration maps purgeInterval and purgeBatchSize to the store options (defaults "1m" and
// 1000). A zero interval disables background purging. The builder registers Close with
// UseShutdownHooks(group) or the process-wide runtime shutdown registry; run shutdown hooks before
// closing the injected database.
package factory
