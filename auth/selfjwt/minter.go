// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Minter issues signed self-issued tokens. It loads a subject's signing key
// from the [KeyProvider] and signs a JWT carrying the subject, a random jti,
// the granted scopes, and the registered temporal claims. The token's kid
// header is the signing key's id so the verifier can resolve the public key and
// a key rotation invalidates the token.
type Minter struct {
	src  KeyProvider
	opts *options
}

// NewMinter builds a Minter over the given key provider.
func NewMinter(src KeyProvider, opts ...Option) *Minter {
	return &Minter{src: src, opts: newOptions(opts...)}
}

// MintRequest is the intent to issue a token for a subject. TTL is clamped to
// the configured maximum token lifetime; a non-positive TTL uses the maximum.
type MintRequest struct {
	Subject string
	Scopes  []string
	TTL     time.Duration
}

// MintResult carries the issued token plus the metadata a caller persists: the
// jti and the absolute expiry.
type MintResult struct {
	Token  string
	ID     string
	Expiry time.Time
}

// Mint issues a signed token for req.Subject. It returns the serialized JWT, its
// jti, and its expiry.
func (m *Minter) Mint(ctx context.Context, req MintRequest) (MintResult, error) {
	start := time.Now()
	res, err := m.mint(ctx, req)
	m.opts.metrics.recordMint(err == nil, time.Since(start))
	return res, err
}

func (m *Minter) mint(ctx context.Context, req MintRequest) (MintResult, error) {
	sk, err := m.src.SigningKey(ctx, req.Subject)
	if err != nil {
		return MintResult{}, err
	}
	if sk.KeyID == "" {
		return MintResult{}, fmt.Errorf("%w: empty key id", ErrSigningKeyInvalid)
	}
	method := jwt.GetSigningMethod(sk.Algorithm.String())
	if method == nil {
		return MintResult{}, fmt.Errorf("%w: %q", ErrAlgorithmNotAllowed, sk.Algorithm)
	}
	jti, err := newID(m.opts.rand)
	if err != nil {
		return MintResult{}, err
	}

	now := m.opts.clock.Now()
	ttl := req.TTL
	if ttl <= 0 || ttl > m.opts.maxTokenLifetime {
		ttl = m.opts.maxTokenLifetime
	}
	exp := now.Add(ttl)

	claims := tokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.opts.issuer,
			Subject:   req.Subject,
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
		Scopes: req.Scopes,
	}
	token := jwt.NewWithClaims(method, claims)
	token.Header["kid"] = sk.KeyID

	raw, err := token.SignedString(sk.Key)
	if err != nil {
		return MintResult{}, fmt.Errorf("selfjwt: sign token: %w", err)
	}
	return MintResult{Token: raw, ID: jti, Expiry: exp}, nil
}

// newID returns a random 128-bit token id as a hex string.
func newID(r io.Reader) (string, error) {
	b := make([]byte, 16)
	if _, err := io.ReadFull(r, b); err != nil {
		return "", fmt.Errorf("selfjwt: generate token id: %w", err)
	}
	return hex.EncodeToString(b), nil
}
