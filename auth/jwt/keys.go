// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package jwt

import (
	"context"
	"crypto"
)

// Header is the subset of a JWT's protected header a [KeyResolver] needs to
// locate the verification key: the signature algorithm and key id.
type Header struct {
	// Alg is the alg header (for example "ES256").
	Alg string
	// Kid is the kid header, the key identifier the token was signed with.
	Kid string
	// Typ is the typ header (commonly "JWT"); empty when the token omits it.
	Typ string
}

// SigningKey is the material [Signer] signs with: a private key, its algorithm,
// and the key id stamped into the token's kid header so a verifier can resolve
// the matching public key.
type SigningKey struct {
	// KeyID is the key identifier written to the token's kid header. It must be
	// non-empty: [Signer.Sign] returns [ErrSigningKeyInvalid] for an empty
	// KeyID, since a verifier rejects a token whose kid header is empty.
	KeyID string
	// Algorithm is the signature algorithm Key is used with.
	Algorithm Algorithm
	// Key is the private key. Its concrete type must match Algorithm as
	// golang-jwt expects: ed25519.PrivateKey for EdDSA, *ecdsa.PrivateKey for
	// ES*, *rsa.PrivateKey for RS*/PS*.
	Key crypto.PrivateKey
}

// VerificationKey is a public key resolved for a token, optionally bound to the
// algorithm it must verify.
type VerificationKey struct {
	// Algorithm, when non-empty, binds the key to one scheme: [Verifier]
	// rejects the token with [ErrAlgorithmNotAllowed] unless it matches the
	// token's alg header, closing key-confusion across algorithms. Leave it
	// empty to let the verifier's allow-list govern the algorithm alone (the
	// usual case when keys come from a JWKS that does not pin alg per key).
	Algorithm Algorithm
	// Key is the public key: ed25519.PublicKey, *ecdsa.PublicKey, or
	// *rsa.PublicKey.
	Key crypto.PublicKey
}

// KeyResolver maps a token to its verification key. It is the single seam
// between this package and a caller's key storage: the resolver sees the
// token's [Header] and its still-unverified [Claims] — enough to resolve by kid
// alone (JWKS style) or by a claim such as sub plus kid (self-issued style) —
// and returns ready crypto material. The signature is verified afterwards, so a
// resolver must not trust the claims beyond using them to locate a key.
//
// Errors are propagated unwrapped by [Verifier.Verify] so a caller can map a
// resolver's own sentinels (for example "subject unknown" or "key rotated").
type KeyResolver interface {
	ResolveKey(ctx context.Context, hdr Header, unverified Claims) (VerificationKey, error)
}

// KeyResolverFunc adapts a function to a [KeyResolver].
type KeyResolverFunc func(ctx context.Context, hdr Header, unverified Claims) (VerificationKey, error)

// ResolveKey calls f.
func (f KeyResolverFunc) ResolveKey(ctx context.Context, hdr Header, unverified Claims) (VerificationKey, error) {
	return f(ctx, hdr, unverified)
}

// StaticKey returns a [KeyResolver] that always resolves to the same key,
// ignoring the header and claims. It suits a single-key deployment or a test.
func StaticKey(vk VerificationKey) KeyResolver {
	return KeyResolverFunc(func(context.Context, Header, Claims) (VerificationKey, error) {
		return vk, nil
	})
}
