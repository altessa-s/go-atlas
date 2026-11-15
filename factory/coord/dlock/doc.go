// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides a fluent builder for creating distributed locks
// from configuration.
//
// [DLockBuilder] uses a fluent API with deferred error accumulation:
// errors from any step are collected and returned at [DLockBuilder.Build] time.
//
//	dl, err := factory.New(cfg.DistributionLock).
//	    UseLogger(logger).
//	    UseProvider(lockconfig.DistributionLockProviderNATS, natsfactory.Builder(natsConn)).
//	    Build(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer dl.Close(ctx)
//
// The builder builds only the sqldb provider; the NATS and MongoDB ones
// register from factory/coord/dlock/providers/nats and
// factory/coord/dlock/providers/mongo, and a configured provider without
// one fails at Build naming the package to add.
package factory
