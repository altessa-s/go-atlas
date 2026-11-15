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
// The builder builds only the in-memory storage. The Redis, NATS and MongoDB
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
package factory
