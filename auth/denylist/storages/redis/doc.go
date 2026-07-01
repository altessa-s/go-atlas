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
// trip, a Store is meant to sit behind a probabilistic negative cache rather
// than be consulted on every request.
//
// # Interfaces
//
// [Store.IsRevoked] gives Store the exact-lookup shape of the negcache
// Authoritative seam, so it can be injected as the authoritative tier behind a
// negative cache. To avoid coupling this backend to the cache package, Store
// does not import negcache; it satisfies that interface structurally.
//
// Store also implements [github.com/altessa-s/go-atlas/data/probfilter.DataLoader]
// via [Store.StreamValues] and [Store.Count], so it can repopulate a negative
// filter from the full set of revoked keys on rebuild.
//
// # Semantics
//
// The write methods mirror the in-memory denylist: [Store.Revoke] denies a key
// permanently, [Store.RevokeUntil] denies it for a bounded TTL (a non-positive
// TTL is a no-op — the token is already invalid on its own), and
// [Store.Restore] removes it.
//
// A Store is safe for concurrent use; go-redis clients are.
//
// # Example
//
//	store := redis.New(client, redis.WithKeyPrefix("denylist:revoked:"))
//	if err := store.Revoke(ctx, "jti-123"); err != nil {
//	    return err
//	}
//	revoked, err := store.IsRevoked(ctx, "jti-123") // true, nil
package redis
