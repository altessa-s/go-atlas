// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package nats stores AIP-155 request IDs in a NATS JetStream KeyValue bucket.
// Each transition checks the entry's owner and caller-supplied time against a KV revision.
//
// Each operation checks ExpiresAt against the caller's now. Configure [WithCleanupGrace]
// above the maximum skew between callers so a leading clock cannot reclaim a live entry.
//
// # Usage
//
//	store, err := nats.New(ctx, js, nats.WithBucket("order_requests"))
//	if err != nil {
//		return err
//	}
//	defer store.Close(ctx)
//	guard, err := requestid.New(store)
package nats
