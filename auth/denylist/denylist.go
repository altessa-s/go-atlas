// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package denylist

import (
	"sync"
	"time"
)

// Checker is the read seam a token verifier consults to reject a revoked
// token. [*Denylist] implements it. Following the project convention, declare
// the dependency on the consumer side and accept a Checker, so a verifier can
// be backed by an in-memory denylist today and a distributed one later without
// changing its code.
type Checker interface {
	// IsRevoked reports whether key — typically a token's JWT ID (jti), or a
	// subject for a blanket ban — has been revoked and has not yet expired.
	IsRevoked(key string) bool
}

// Denylist is a concurrency-safe set of revoked token identifiers. A token
// package consults it by the token's jti (or subject) to reject tokens that
// were explicitly revoked before their natural expiry — the reusable seam the
// token packages (auth/jwt, auth/oidc, selfjwt) otherwise lack, each carrying
// its own ad-hoc revocation or none at all.
//
// Entries are either permanent ([Denylist.Revoke]) or bounded to an expiry
// ([Denylist.RevokeUntil]). Expired entries are swept on writes and ignored on
// reads, so a denylist that only ever revokes until each token's own expiry
// stays bounded without a background goroutine. Unlike a cache it never evicts
// a live (unexpired) entry — dropping one would silently un-revoke a token — so
// growth is bounded by TTL expiry and the caller's use of permanent
// revocations, never by capacity.
//
// The zero value is not usable; construct one with [New].
type Denylist struct {
	now func() time.Time

	mu      sync.RWMutex
	entries map[string]time.Time // key -> expiry; the zero time means permanent.
}

// New returns an empty Denylist.
func New(opts ...Option) *Denylist {
	d := &Denylist{
		now:     time.Now,
		entries: make(map[string]time.Time),
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// Revoke denies key permanently, until a matching [Denylist.Restore]. Use it
// for identifiers that must never be accepted again, such as a compromised
// subject whose every token should be rejected.
func (d *Denylist) Revoke(key string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.sweepLocked()
	d.entries[key] = time.Time{}
}

// RevokeUntil denies key until expiry, after which it is forgotten. Pass the
// token's own expiration so the entry is retained no longer than necessary. An
// expiry at or before now is a no-op — the token is already invalid on its own.
func (d *Denylist) RevokeUntil(key string, expiry time.Time) {
	if !expiry.After(d.now()) {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.sweepLocked()
	d.entries[key] = expiry
}

// Restore removes key from the denylist, re-allowing it.
func (d *Denylist) Restore(key string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.entries, key)
}

// IsRevoked reports whether key is currently revoked. An entry past its expiry
// reports false; it is reclaimed on the next write or [Denylist.Sweep].
func (d *Denylist) IsRevoked(key string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	expiry, ok := d.entries[key]
	if !ok {
		return false
	}
	return expiry.IsZero() || d.now().Before(expiry)
}

// Len returns the number of tracked entries, including any expired ones not yet
// swept. Intended for observability and tests.
func (d *Denylist) Len() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.entries)
}

// Sweep removes all expired entries and returns the number dropped. Writes
// sweep automatically; call it explicitly only to reclaim memory from expired
// entries once revocations have stopped arriving.
func (d *Denylist) Sweep() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.sweepLocked()
}

// sweepLocked deletes expired entries and returns how many were removed. The
// caller holds d.mu.
func (d *Denylist) sweepLocked() int {
	now := d.now()
	var n int
	for key, expiry := range d.entries {
		if !expiry.IsZero() && !now.Before(expiry) {
			delete(d.entries, key)
			n++
		}
	}
	return n
}
