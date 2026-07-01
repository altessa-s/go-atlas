// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/altessa-s/go-atlas/auth/jwt"
)

// Minter issues signed self-issued tokens. It loads a subject's signing key
// from the [KeyProvider] and signs a JWT carrying the subject, a random jti,
// the granted scopes, and the registered temporal claims. The token's kid
// header is the signing key's id so the verifier can resolve the public key and
// a key rotation invalidates the token. The signing crypto and algorithm
// allow-list are delegated to [github.com/altessa-s/go-atlas/auth/jwt.Signer].
type Minter struct {
	src    KeyProvider
	signer *jwt.Signer
	opts   *options
}

// NewMinter builds a Minter over the given key provider.
func NewMinter(src KeyProvider, opts ...Option) *Minter {
	o := newOptions(opts...)
	return &Minter{
		src:    src,
		signer: jwt.NewSigner(jwt.WithAllowedAlgorithms(o.allowedAlgorithms...)),
		opts:   o,
	}
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
	// A subject is mandatory: an empty sub yields a token the verifier always
	// rejects (resolveKey requires a non-empty subject), so fail at mint time
	// rather than emit an unverifiable token.
	if req.Subject == "" {
		return MintResult{}, ErrSubjectRequired
	}
	sk, err := m.src.SigningKey(ctx, req.Subject)
	if err != nil {
		return MintResult{}, err
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

	claims := jwt.Claims{
		"sub": req.Subject,
		"jti": jti,
		"iat": now.Unix(),
		"nbf": now.Unix(),
		"exp": exp.Unix(),
	}
	if m.opts.issuer != "" {
		claims["iss"] = m.opts.issuer
	}
	if len(req.Scopes) > 0 {
		claims["scope"] = req.Scopes
	}

	raw, err := m.signer.Sign(sk, claims)
	if err != nil {
		return MintResult{}, mapSignErr(err)
	}
	return MintResult{Token: raw, ID: jti, Expiry: exp}, nil
}

// mapSignErr translates the auth/jwt signing sentinels back onto the selfjwt
// contract so callers keep matching [ErrSigningKeyInvalid] / [ErrAlgorithmNotAllowed].
func mapSignErr(err error) error {
	switch {
	case errors.Is(err, jwt.ErrSigningKeyInvalid):
		return fmt.Errorf("%w: %w", ErrSigningKeyInvalid, err)
	case errors.Is(err, jwt.ErrAlgorithmNotAllowed):
		return fmt.Errorf("%w: %w", ErrAlgorithmNotAllowed, err)
	default:
		// Any other signing failure (bad key type, entropy failure) still maps to
		// the documented sentinel so callers can match ErrSigningKeyInvalid.
		return fmt.Errorf("%w: %w", ErrSigningKeyInvalid, err)
	}
}

// newID returns a random 128-bit token id as a hex string.
func newID(r io.Reader) (string, error) {
	b := make([]byte, 16)
	if _, err := io.ReadFull(r, b); err != nil {
		return "", fmt.Errorf("selfjwt: generate token id: %w", err)
	}
	return hex.EncodeToString(b), nil
}
