// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package leadelect provides distributed leader election for service coordination.
// Supports multiple providers (NATS, Redis) for electing a single leader among instances
// to perform singleton tasks in a distributed environment.
//
// # Features
//
//   - Single leader guarantee: ensures only one instance is active as leader.
//   - Callback system: notification when becoming or losing leadership.
//   - Pluggable backends: NATS JetStream, Redis.
//   - Automatic re-election: handles node failures gracefully.
//
// # Usage
//
//	provider, _ := nats.NewProvider(conn, nats.WithBucket("leaders"))
//	le := leadelect.New(provider,
//	    leadelect.WithOnBecomeLeader(func() {
//	        log.Println("Acquired leadership, starting background tasks...")
//	    }),
//	    leadelect.WithOnEvicted(func() {
//	        log.Println("Lost leadership, stopping background tasks...")
//	    }),
//	)
//	le.Start(ctx)
package leadelect
