// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package nats provides NATS JetStream KeyValue storage for distributed unique values.
// Requires NATS Server v2.11.0+ with JetStream enabled. Supports configurable bucket, TTL, and storage type (memory/file).
//
// Example:
//
//	nc, _ := nats.Connect("nats://localhost:4222")
//	provider, _ := nats.New(nc, nats.WithBucket("myapp"), nats.WithTTL(30*time.Second))
//	u := uniq.New(provider)
//	u.Add(ctx, "user:123")
package nats
