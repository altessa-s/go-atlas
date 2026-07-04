// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package mirror adapts an asynchronous, distributed revocation store to the
// synchronous [github.com/altessa-s/go-atlas/auth/jwt.RevocationChecker] seam
// that auth/jwt and auth/selfjwt verifiers accept.
//
// # Why
//
// jwt.RevocationChecker is a hot-path, synchronous IsRevoked(id) bool — no
// context, no error — so it cannot make a network call on every verification.
// The distributed denylist store
// ([github.com/altessa-s/go-atlas/auth/denylist/storages/redis.Store]) is
// asynchronous (IsRevoked(ctx, key) (bool, error)) and therefore does not fit.
// Mirror bridges the two: it keeps a local snapshot of the revoked-key set,
// refreshed on a schedule, and answers every lookup from memory.
//
// # Model
//
// [Cache.Refresh] streams the authoritative store's current revoked keys
// through the narrow [Source] seam and atomically swaps in a fresh snapshot.
// [Cache.IsRevoked] reads that snapshot lock-free, so the verifier hot path
// never touches the network and never blocks.
//
// # Staleness
//
// A snapshot lags the authoritative store by up to one refresh interval: a key
// revoked on another node is not seen until the next Refresh — the same
// propagation window any locally cached revocation set has. Size the refresh
// cadence to your revocation-propagation SLA. Because the store expires
// TTL-bounded revocations natively, a refreshed snapshot only ever shrinks as
// entries expire; it never resurrects a key the store has dropped.
//
// # Usage
//
//	m := mirror.New(store)                 // store satisfies mirror.Source
//	if err := m.Refresh(ctx); err != nil { // prime before serving
//	    return err
//	}
//	v := jwt.NewVerifier(resolver, jwt.WithRevocation(m))
//
//	// Keep it fresh on a schedule (core/scheduler, a ticker, …):
//	_ = registrar.Register("denylist-mirror", m.Refresh)
package mirror
