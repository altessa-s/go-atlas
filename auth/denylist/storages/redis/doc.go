// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package redis provides a Redis-backed distributed exact revocation store for
// the denylist subsystem.
//
// # Role
//
// A [Store] is the authoritative, cross-node source of truth for revoked keys —
// token IDs (jti) or subjects. Each key maps to a single Redis string whose
// presence means "revoked"; Redis's native TTL handling expires bounded
// revocations without a background sweeper. Because lookups cost a network round
// trip, consumers can use a negative cache when eventual consistency suffices.
//
// # Reads and rebuilds
//
// Call [Store.IsRevoked] directly for exact reads on an uncached primary.
// [WithStrictExactReads] checks the serving role in the same read-only script
// as the lookup. Durability across failover depends on the Redis deployment.
//
// [Store.RawSource] streams bare keys for a mirror. [Store.DataLoader] supplies
// the same keys to a negative filter. Both verify the pinned standalone primary
// and its process ID on every scan page. Drift aborts the rebuild.
//
// # Retention
//
// [Store.Revoke] denies permanently. [Store.RevokeUntil] only extends existing
// revocations, rounding positive durations up to milliseconds. [Store.Restore]
// removes either kind of revocation.
//
// // # Example
//
//	store, err := redis.New(client, redis.WithKeyPrefix("denylist:revoked:"))
//	if err != nil {
//	    return err
//	}
//	if err := store.Revoke(ctx, "jti-123"); err != nil {
//	    return err
//	}
//	revoked, err := store.IsRevoked(ctx, "jti-123") // true, nil
package redis
