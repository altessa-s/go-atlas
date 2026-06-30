// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package denylist provides a reusable token-revocation seam shared across the
// token packages. It is a concurrency-safe set of revoked identifiers — a JWT
// ID (jti) for a single token, or a subject for a blanket ban — that a verifier
// consults to reject tokens revoked before their natural expiry.
//
// # Why
//
// auth/oidc carries its own revocation (introspection plus a probabilistic
// filter) and selfjwt only rotates keys; nothing offered a small denylist seam
// the token packages could share. This package is that primitive: transport-
// and JWT-free, keyed by a plain string, so any token package can adopt it.
//
// # Bounding
//
// Entries are permanent ([Denylist.Revoke]) or expiry-bounded
// ([Denylist.RevokeUntil]); expired entries are swept on writes and ignored on
// reads. Unlike a cache it never evicts a live entry, which would silently
// un-revoke a token — growth is bounded by TTL expiry, not capacity.
//
// # Usage
//
//	dl := denylist.New()
//	dl.RevokeUntil(claims.ID(), claims.ExpiresAt()) // deny this token until it expires anyway
//
//	if dl.IsRevoked(claims.ID()) {
//	    return ErrRevoked
//	}
//
// Wire it on the consumer side through the [Checker] seam so a verifier can be
// backed by an in-memory denylist now and a distributed one later.
package denylist
