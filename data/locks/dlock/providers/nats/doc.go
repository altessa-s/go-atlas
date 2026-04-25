// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package nats provides NATS JetStream KeyValue-based distributed locks with automatic renewal.
// Implements optimistic concurrency control with stale lock recovery. Requires NATS Server v2.11.0+.
//
// Example:
//
//	nc, _ := nats.Connect("nats://localhost:4222")
//	provider, _ := nats.New(nc,
//		nats.WithBucket("my-locks"),
//		nats.WithTTL(30*time.Second),
//	)
//	dl := dlock.New(provider)
//	defer dl.Close()
//	lock, _ := dl.Lock(ctx, "resource-key")
//	defer lock.Release(ctx)
package nats
