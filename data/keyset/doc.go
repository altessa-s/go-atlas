// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package keyset issues and resolves signed, stateless page tokens for
// keyset (cursor) pagination.
//
// A token carries an opaque position payload chosen by the caller (for
// example "timestamp|id" of the last row of a page), its issue time and
// digests of the [Bindings] — fingerprints of the sort, the filter and the
// principal — all signed with HMAC-SHA256. [Codec.Resolve] checks the
// signature first, then expiry and the bindings, so a token cannot be forged,
// replayed after [DefaultTTL], replayed under another filter or sort, or used
// by another subject.
//
// The signing key is required, at least [MinKeyLength] bytes, and must be the
// same on every replica; [WithPreviousKeys] keeps tokens signed before a key
// rotation valid. Tokens are not stored and cannot be revoked individually.
//
//	codec, err := keyset.New(signingKey)
//	token, err := codec.Issue([]byte(position), keyset.Bindings{Sort: sortFP, Filter: filterFP, Subject: user})
//	position, err := codec.Resolve(token, keyset.Bindings{Sort: sortFP, Filter: filterFP, Subject: user})
package keyset
