// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package jwt

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Audience is the aud claim. A JWT audience may be encoded as a single string
// or an array of strings; UnmarshalJSON accepts both and MarshalJSON always
// emits an array, so a service struct can declare it as one type.
type Audience []string

// UnmarshalJSON accepts a JSON string or array of strings.
func (a *Audience) UnmarshalJSON(data []byte) error {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	out, _ := normalizeStringList(v)
	*a = out
	return nil
}

// MarshalJSON emits the audience as a JSON array.
func (a Audience) MarshalJSON() ([]byte, error) {
	return json.Marshal([]string(a))
}

// ClaimsBase carries the registered JWT claims a service's typed claim struct
// embeds to use [SignStruct] / [VerifyInto]. Embedding it is mandatory: the
// generics accept only types that embed ClaimsBase (enforced at compile time by
// the unexported marker method). Custom claims are added as further fields with
// json tags on the embedding struct.
//
//	type MyClaims struct {
//	    jwt.ClaimsBase
//	    Role string `json:"role"`
//	}
type ClaimsBase struct {
	Issuer    string   `json:"iss,omitempty"`
	Subject   string   `json:"sub,omitempty"`
	Audience  Audience `json:"aud,omitempty"`
	ExpiresAt int64    `json:"exp,omitempty"`
	NotBefore int64    `json:"nbf,omitempty"`
	IssuedAt  int64    `json:"iat,omitempty"`
	ID        string   `json:"jti,omitempty"`
}

// structuredClaimsMarker has a value receiver so it is promoted to both the
// embedding value and its pointer, letting [SignStruct] (value) and
// [VerifyInto] (pointer) both accept embedders. It is unexported so only types
// that embed [ClaimsBase] satisfy [structuredClaims].
func (ClaimsBase) structuredClaimsMarker() {}

// ExpiryTime returns the exp claim as a time, or the zero time when unset.
func (b ClaimsBase) ExpiryTime() time.Time { return unixOrZero(b.ExpiresAt) }

// NotBeforeTime returns the nbf claim as a time, or the zero time when unset.
func (b ClaimsBase) NotBeforeTime() time.Time { return unixOrZero(b.NotBefore) }

// IssuedAtTime returns the iat claim as a time, or the zero time when unset.
func (b ClaimsBase) IssuedAtTime() time.Time { return unixOrZero(b.IssuedAt) }

func unixOrZero(sec int64) time.Time {
	if sec == 0 {
		return time.Time{}
	}
	return time.Unix(sec, 0).UTC()
}

// structuredClaims is satisfied only by types embedding [ClaimsBase].
type structuredClaims interface {
	structuredClaimsMarker()
}

// VerifyInto verifies raw with v and decodes the validated claims into a new T,
// a struct embedding [ClaimsBase]. It runs the full [Verifier.Verify] pipeline
// (signature, algorithm, temporal/issuer/audience/required-claim and typ
// checks) and then maps the claim set onto T's json-tagged fields, so the same
// errors as [Verifier.Verify] apply.
//
//	out, err := jwt.VerifyInto[MyClaims](ctx, verifier, raw) // *MyClaims
func VerifyInto[T any, PT interface {
	*T
	structuredClaims
}](ctx context.Context, v *Verifier, raw string) (PT, error) {
	claims, err := v.Verify(ctx, raw)
	if err != nil {
		return nil, err
	}
	out := PT(new(T))
	if err := claims.Decode(out); err != nil {
		return nil, err
	}
	return out, nil
}

// SignStruct signs a typed claim struct embedding [ClaimsBase] with key. The
// struct is marshaled to a [Claims] set and handed to [Signer.Sign], so the
// same algorithm allow-list, kid header, and typ behavior apply.
//
//	raw, err := jwt.SignStruct(signer, key, MyClaims{ClaimsBase: base, Role: "admin"})
func SignStruct[T structuredClaims](s *Signer, key SigningKey, claims T) (string, error) {
	m, err := toClaims(claims)
	if err != nil {
		return "", err
	}
	return s.Sign(key, m)
}

// toClaims converts a struct to a [Claims] set via a JSON round-trip so its
// json tags drive the claim names.
func toClaims(v any) (Claims, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("auth/jwt: encode struct claims: %w", err)
	}
	var c Claims
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("auth/jwt: decode struct claims into map: %w", err)
	}
	return c, nil
}

// NewBase builds a [ClaimsBase] carrying the registered temporal claims (iat,
// nbf, exp), the subject, a random jti, and the signer's issuer, using the
// signer's clock and max lifetime. A non-positive ttl, or one above the
// configured maximum, is clamped to [DefaultMaxTokenLifetime] (or the value
// from [WithMaxTokenLifetime]). It is the typed counterpart of
// [Signer.NewClaims]; embed the result in a service struct before [SignStruct].
func (s *Signer) NewBase(subject string, ttl time.Duration) (ClaimsBase, error) {
	jti, err := newID(s.opts.rand)
	if err != nil {
		return ClaimsBase{}, err
	}
	now := s.opts.clock.Now()
	if ttl <= 0 || ttl > s.opts.maxTokenLifetime {
		ttl = s.opts.maxTokenLifetime
	}
	return ClaimsBase{
		Issuer:    s.opts.issuer,
		Subject:   subject,
		IssuedAt:  now.Unix(),
		NotBefore: now.Unix(),
		ExpiresAt: now.Add(ttl).Unix(),
		ID:        jti,
	}, nil
}
