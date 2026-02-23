// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides configuration-based creation of Redis clients.
//
// It integrates with [config.Redis] to create [redis.UniversalClient]
// instances with authentication, connection pooling, and support for
// standalone, sentinel, and cluster modes. The mode is determined
// automatically: sentinel when [config.Redis.MasterName] is set, cluster
// when multiple hosts are provided, standalone otherwise.
//
// The factory supports two configuration paths: connection-URI based
// (when [config.Redis.ConnectionURI] is set) and field-based (individual
// host, credential, and pool settings). Both paths converge in
// [Factory.UniversalOptionsFromConfig].
//
// When a [health.Coordinator] is provided via [WithHealthCoordinator],
// the factory registers a health checker that pings Redis on each check.
//
// Example:
//
//	f := factory.New(factory.WithLogger(logger))
//	client, err := f.CreateClientFromConfig(ctx, cfg.Redis)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer client.Close()
package factory
