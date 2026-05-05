// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package pool provides gRPC client connection pooling with automatic cleanup and health monitoring.
//
// Each target address gets its own sub-pool of connections. A background goroutine
// periodically evicts idle and unhealthy connections based on configurable thresholds.
//
// Callers must return every connection obtained via [ConnectionPool.GetConnection]
// by calling [ConnectionPool.ReturnConnection]; failing to do so leaks connections.
//
// # Lifecycle
//
//	p := pool.New(pool.WithSize(20), pool.WithMaxIdleTime(time.Hour))
//	stop, err := p.Start(ctx)
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer stop()
//
//	conn, err := p.GetConnection(ctx, "localhost:8080")
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer p.ReturnConnection(conn)
//
// After calling stop (or canceling the context passed to [ConnectionPool.Start]),
// all connections are closed and further calls to [ConnectionPool.GetConnection]
// return [ErrConnectionPoolClosed].
//
// # Health integration
//
// Pass [WithHealthCoordinator] to register an aggregate [health.Checker]
// reporting the worst per-target status across the pool. When
// [WithPerTargetHealthChecks] is set, a `<service>.<target>` checker is
// registered lazily on the first conn for a target and removed when its
// last conn is closed. The mapping from gRPC [connectivity.State] to
// [health.ServingStatus] uses [DefaultStateMapper] (override with
// [WithHealthStateMapper]).
//
// External consumers can observe state without a coordinator via the
// public [ConnectionPool.SubscribeTarget] / [ConnectionPool.StateForTarget]
// API; the gRPC client uses these in pool mode.
package pool
