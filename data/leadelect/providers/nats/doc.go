// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package nats provides NATS JetStream KeyValue-based leader election for distributed systems.
// Uses TTL-based leadership with automatic renewal and failure recovery. Requires NATS Server v2.11.0+.
//
// Example:
//
//	conn, _ := nats.Connect("nats://localhost:4222")
//	provider, _ := nats.New(ctx, conn, nats.WithLogger(logger))
//	le := leadelect.New(provider, "my-service-leader", "instance-1", leadelect.WithTTL(30*time.Second))
//	le.Start(ctx)
package nats
