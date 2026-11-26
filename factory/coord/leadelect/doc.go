// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides a fluent builder for creating leader electors
// from configuration.
//
// [LeaderBuilder] uses a fluent API with deferred error accumulation:
// errors from any step are collected and returned at [LeaderBuilder.Build] time.
// Build requires an election key and a node ID before provisioning the provider.
//
//	le, err := factory.New(cfg.LeaderElector).
//	    UseLogger(logger).
//	    UseProvider(lockconfig.LeaderElectorProviderNATS, natsfactory.Builder(natsConn)).
//	    WithKey("my-app").
//	    WithNodeId("node-1").
//	    Build(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer le.Stop(ctx)
//
// The builder builds no provider itself; the NATS one registers from
// factory/coord/leadelect/providers/nats, and a configured provider without
// one fails at Build naming the package to add. An empty provider is nats.
package factory
