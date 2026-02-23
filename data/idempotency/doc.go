// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package idempotency provides duplicate request detection using idempotency keys.
// Supports multiple storage backends (memory, Redis, NATS) for distributed systems.
//
// Example:
//
//	storage := memory.New(memory.WithTTL(time.Hour))
//	keeper := idempotency.New(storage)
//	isDuplicate, err := keeper.Check(ctx, "request-123")
//	if isDuplicate {
//	    // Return cached response
//	}
package idempotency
