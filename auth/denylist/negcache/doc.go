// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package negcache places a probabilistic negative cache in front of an
// authoritative — typically distributed — revocation store, so the common case
// (a token that was never revoked) is answered locally without a network round
// trip.
//
// # Model
//
// A [github.com/altessa-s/go-atlas/data/probfilter.Filter] holds a superset of
// the revoked keys. On a lookup:
//
//   - the filter reports the key definitely absent → the key is not revoked,
//     answered locally (the fast path);
//   - the filter reports the key possibly present, or the filter errors → the
//     [Authoritative] store is consulted for the exact answer.
//
// Because a Bloom/Cuckoo filter has no false negatives for keys it has seen, a
// revoked key is never fast-pathed as absent — provided the filter is kept a
// superset of the revoked set (see the invariant below). Filter false positives
// only cost an extra authoritative lookup; they never let a revoked token
// through and never reject a valid one, since the authoritative store makes the
// final call.
//
// # Invariant and staleness
//
// The negative cache is correct only while the filter contains every key the
// authoritative store considers revoked. Maintain it two ways:
//
//   - call [Cache.Add] whenever this node revokes a key, and
//   - call [Cache.Rebuild] on a schedule to repopulate the filter from the
//     authoritative store's full key stream — this absorbs revocations made on
//     other nodes and reclaims a Bloom filter whose false-positive rate has
//     grown.
//
// Between rebuilds, a key revoked on another node is not yet in this node's
// filter, so it is fast-pathed as not-revoked until the next rebuild. This is
// the same propagation window any locally cached revocation set has; size the
// rebuild cadence to the revocation-propagation SLA the deployment requires.
//
// # Usage
//
//	filter, _ := probfilterfactory.NewFilter("denylist", cfg, &defaults).Build(ctx)
//	cache := negcache.New(filter, redisDenylist) // redisDenylist is Authoritative
//	// Consult on the hot path:
//	revoked, err := cache.IsRevoked(ctx, jti)
//	// Repopulate periodically from the authoritative store:
//	_ = scheduler.Register("denylist-rebuild", func(ctx context.Context) error {
//	    return cache.Rebuild(ctx, loader)
//	})
package negcache
