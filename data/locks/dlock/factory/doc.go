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
//	    UseNatsConn(natsConn).
//	    Build(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer dl.Close(ctx)
package factory
