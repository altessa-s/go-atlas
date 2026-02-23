// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides configuration-based creation of leader electors.
// It integrates with the config.LeaderElector configuration to create
// Leader instances with their respective providers.
//
// Example:
//
//	f := factory.New(
//	    factory.WithKey("my-app"),
//	    factory.WithNodeId("node-1"),
//	    factory.WithNatsConn(natsConn),
//	)
//	le, err := f.CreateLeaderFromConfig(ctx, cfg)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer le.Stop(ctx)
package factory
