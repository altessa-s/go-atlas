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
package pool
