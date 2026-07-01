// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt

import (
	"context"

	"github.com/altessa-s/go-atlas/auth/jwt"
)

// SigningKey aliases [github.com/altessa-s/go-atlas/auth/jwt.SigningKey]: a
// subject's current signing material — a private key, its algorithm, and the key
// id stamped into the token header so the verifier can resolve the matching
// public key. KeyID must be non-empty: [Minter.Mint] returns
// [ErrSigningKeyInvalid] for an empty KeyID, since the verifier rejects a token
// whose kid header is empty.
type SigningKey = jwt.SigningKey

// VerificationKey aliases [github.com/altessa-s/go-atlas/auth/jwt.VerificationKey]:
// a subject's public key for a given kid together with the algorithm it must be
// used with. selfjwt requires Algorithm to be set — the verifier rejects a key
// with an empty Algorithm as [ErrAlgorithmNotAllowed], binding each key to one
// scheme.
type VerificationKey = jwt.VerificationKey

// KeyProvider supplies a subject's signing and verification keys. It is the
// single seam between selfjwt and a caller's key storage; selfjwt never sees
// the underlying key format (seed, PEM, KMS handle) — the provider returns
// ready crypto keys.
//
// The package is decoupled from any particular store via this interface, which
// keeps the minter and verifier unit-testable with a fake.
type KeyProvider interface {
	// SigningKey returns the subject's current signing material. Implementations
	// return [ErrSubjectUnknown] when the subject has no keys.
	SigningKey(ctx context.Context, subject string) (SigningKey, error)

	// VerificationKey resolves a token's kid to the subject's public key,
	// accepting the current signing key and any retired-but-valid key still in
	// the rotation overlap window. Implementations return [ErrSubjectUnknown]
	// when the subject is unknown and [ErrKeyRotated] when the subject is known
	// but the kid is not.
	VerificationKey(ctx context.Context, subject, kid string) (VerificationKey, error)
}
