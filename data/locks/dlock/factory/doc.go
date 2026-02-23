// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides configuration-based creation of distributed locks.
// It integrates with the config.DistributionLock configuration to create
// DLock instances with their respective providers.
//
// Example:
//
//	f := factory.New(factory.WithNatsConn(conn), factory.WithLogger(logger))
//	dl, err := f.CreateDLockFromConfig(ctx, cfg)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer dl.Close(ctx)
package factory
