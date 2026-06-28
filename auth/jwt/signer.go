// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package jwt

import (
	"encoding/hex"
	"fmt"
	"io"
	"slices"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"
)

// JWT typ header values.
const (
	// TypeJWT is the default typ golang-jwt writes when none is set.
	TypeJWT = "JWT"
	// TypeAccessToken is the typ for OAuth 2.0 JWT access tokens (RFC 9068):
	// "at+jwt". Pass it to [WithTokenType] when signing access tokens and to
	// [WithExpectedTokenType] when validating them.
	TypeAccessToken = "at+jwt"
)

// Signer issues signed JWTs. It signs a caller-supplied [Claims] set with a
// [SigningKey], stamping the key's id into the kid header so a verifier can
// resolve the matching public key. The signing algorithm must be in the
// allow-list (asymmetric only by default), the same posture as verification.
type Signer struct {
	opts *options
}

// NewSigner builds a Signer with the given options.
func NewSigner(opts ...Option) *Signer {
	return &Signer{opts: newOptions(opts...)}
}

// NewClaims builds a claim set carrying the registered temporal claims (iat,
// nbf, exp), the subject, a random jti, the signer's issuer, and any scopes,
// using the signer's clock and max lifetime. A non-positive ttl, or one above
// the configured maximum, is clamped to [DefaultMaxTokenLifetime] (or the value
// from [WithMaxTokenLifetime]). The caller may add or override claims with
// [Claims.Set] before signing.
func (s *Signer) NewClaims(subject string, ttl time.Duration, scopes ...string) (Claims, error) {
	jti, err := newID(s.opts.rand)
	if err != nil {
		return nil, err
	}
	now := s.opts.clock.Now()
	if ttl <= 0 || ttl > s.opts.maxTokenLifetime {
		ttl = s.opts.maxTokenLifetime
	}
	c := Claims{
		"sub": subject,
		"jti": jti,
		"iat": now.Unix(),
		"nbf": now.Unix(),
		"exp": now.Add(ttl).Unix(),
	}
	if s.opts.issuer != "" {
		c["iss"] = s.opts.issuer
	}
	if len(scopes) > 0 {
		c["scope"] = slices.Clone(scopes)
	}
	return c, nil
}

// Sign serializes and signs claims with key. It returns [ErrSigningKeyInvalid]
// for an empty key id and [ErrAlgorithmNotAllowed] when the key's algorithm is
// outside the allow-list or unknown to golang-jwt.
func (s *Signer) Sign(key SigningKey, claims Claims) (string, error) {
	if key.KeyID == "" {
		return "", fmt.Errorf("%w: empty key id", ErrSigningKeyInvalid)
	}
	if !slices.Contains(s.opts.allowedAlgorithms, key.Algorithm) {
		return "", fmt.Errorf("%w: %q", ErrAlgorithmNotAllowed, key.Algorithm)
	}
	method := gojwt.GetSigningMethod(key.Algorithm.String())
	if method == nil {
		return "", fmt.Errorf("%w: %q", ErrAlgorithmNotAllowed, key.Algorithm)
	}
	token := gojwt.NewWithClaims(method, gojwt.MapClaims(claims))
	token.Header["kid"] = key.KeyID
	if s.opts.tokenType != "" {
		token.Header["typ"] = s.opts.tokenType
	}
	raw, err := token.SignedString(key.Key)
	if err != nil {
		return "", fmt.Errorf("auth/jwt: sign token: %w", err)
	}
	return raw, nil
}

// newID returns a random 128-bit token id as a hex string.
func newID(r io.Reader) (string, error) {
	b := make([]byte, 16)
	if _, err := io.ReadFull(r, b); err != nil {
		return "", fmt.Errorf("auth/jwt: generate token id: %w", err)
	}
	return hex.EncodeToString(b), nil
}
