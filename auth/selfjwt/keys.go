// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt

import (
	"context"
	"crypto"
)

// SigningKey is a subject's current signing material: a private key, its
// algorithm, and the key id stamped into the token header so the verifier can
// resolve the matching public key.
type SigningKey struct {
	// KeyID is the key identifier written to the token's "kid" header. It must
	// be non-empty: [Minter.Mint] returns [ErrSigningKeyInvalid] for an empty
	// KeyID, since the verifier rejects a token whose kid header is empty.
	KeyID string
	// Algorithm is the signature algorithm Key is used with.
	Algorithm Algorithm
	// Key is the private key. Its concrete type must match Algorithm as
	// golang-jwt expects: ed25519.PrivateKey for EdDSA, *ecdsa.PrivateKey for
	// ES*, *rsa.PrivateKey for RS*/PS*.
	Key crypto.PrivateKey
}

// VerificationKey is a subject's public key for a given kid, together with the
// algorithm it must be used with.
type VerificationKey struct {
	// Algorithm is the signature algorithm Key verifies. It is required: the
	// verifier rejects the token with [ErrAlgorithmNotAllowed] when it is empty
	// or does not match the token's algorithm, binding each key to one scheme.
	Algorithm Algorithm
	// Key is the public key: ed25519.PublicKey, *ecdsa.PublicKey, or
	// *rsa.PublicKey.
	Key crypto.PublicKey
}

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
