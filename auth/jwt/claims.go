// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package jwt

import (
	"encoding/json"
	"fmt"
	"iter"
	"slices"
	"time"

	"github.com/altessa-s/go-atlas/auth/internal/claimscope"
	"github.com/altessa-s/go-atlas/auth/internal/numericdate"
)

const (
	claimExpiration = "exp"
	claimIssuedAt   = "iat"
	claimNotBefore  = "nbf"
)

// Claims is a JWT claim set: the decoded payload as a map plus typed accessors
// for the registered claims a caller usually needs. It is the value [Signer]
// signs and the value [Verifier] returns. A richer principal model (roles,
// tenancy, superadmin flags) is the caller's concern and is built on top of
// the raw claims read through these accessors.
type Claims map[string]any

// Set assigns a claim and returns the receiver so calls chain. It is a
// convenience for assembling a claim set before signing.
func (c Claims) Set(name string, value any) Claims {
	c[name] = value
	return c
}

// Has reports whether the named claim is present and non-null. A JSON null
// decodes to a nil map entry, which carries no value, so it is treated as
// absent: this keeps [WithRequiredClaims] honest, rejecting a token that sends
// a required claim as null.
func (c Claims) Has(name string) bool {
	v, ok := c[name]
	return ok && v != nil
}

// Issuer returns the iss claim, or "" when absent or not a string.
func (c Claims) Issuer() string { s, _ := c.String("iss"); return s }

// Subject returns the sub claim, or "" when absent or not a string.
func (c Claims) Subject() string { s, _ := c.String("sub"); return s }

// ID returns the jti claim, or "" when absent or not a string.
func (c Claims) ID() string { s, _ := c.String("jti"); return s }

// Audience returns the aud claim normalized to a slice. A JWT aud may be a
// single string or an array; both yield a slice (empty when absent).
func (c Claims) Audience() []string {
	v, ok := c["aud"]
	if !ok {
		return nil
	}
	out, _ := normalizeStringList(v)
	return out
}

// Expiry rounds exp up to the next representable instant for retention deadlines.
// An absent or invalid date yields the zero time.
func (c Claims) Expiry() time.Time {
	d, ok := numericdate.Parse(c[claimExpiration])
	if !ok {
		return time.Time{}
	}
	return d.Expiry()
}

// NotBefore returns the nbf claim as a time, or the zero time when absent.
func (c Claims) NotBefore() time.Time { return c.Time(claimNotBefore) }

// IssuedAt returns the iat claim as a time, or the zero time when absent.
func (c Claims) IssuedAt() time.Time { return c.Time(claimIssuedAt) }

// Scopes returns the scope claim normalized to a slice. The scope claim may be
// a space-separated string (OAuth 2.0 style) or an array; both yield a slice.
func (c Claims) Scopes() []string {
	v, ok := c["scope"]
	if !ok {
		return nil
	}
	return slices.Collect(scopeSeq(v))
}

// String returns the named claim as a string. The bool is false when the claim
// is absent or not a string.
func (c Claims) String(name string) (string, bool) {
	s, ok := c[name].(string)
	return s, ok
}

// StringSlice returns the named claim normalized to a string slice, accepting a
// single string, a []string, or a []any of strings. The bool is false when the
// claim is absent or of an unsupported type.
func (c Claims) StringSlice(name string) ([]string, bool) {
	v, ok := c[name]
	if !ok {
		return nil, false
	}
	return normalizeStringList(v)
}

// Time returns the date rounded down to a nanosecond, or the zero time if
// the named claim is absent or invalid.
func (c Claims) Time(name string) time.Time {
	d, ok := numericdate.Parse(c[name])
	if !ok {
		return time.Time{}
	}
	return d.Time()
}

// Decode unmarshals the whole claim set into target, a pointer to a
// service-defined struct whose json tags name the claims to read (registered
// and custom alike). It round-trips through JSON, so the same numeric and time
// conventions as standard json.Unmarshal apply.
//
//	type MyClaims struct {
//	    Subject string   `json:"sub"`
//	    Role    string   `json:"role"`
//	    Level   int      `json:"level"`
//	    Groups  []string `json:"groups"`
//	}
//	var mc MyClaims
//	err := claims.Decode(&mc)
func (c Claims) Decode(target any) error {
	data, err := json.Marshal(map[string]any(c))
	if err != nil {
		return fmt.Errorf("auth/jwt: encode claims: %w", err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("auth/jwt: decode claims into %T: %w", target, err)
	}
	return nil
}

// Get returns the named claim asserted to T. The bool is false when the claim
// is absent or not of type T. It is a free function because Go methods cannot
// take type parameters. Verified registered dates use json.Number. Read a
// numeric custom claim with Get[float64], a flag with Get[bool], or a set of
// fields with [Claims.Decode].
func Get[T any](c Claims, name string) (T, bool) {
	v, ok := c[name].(T)
	return v, ok
}

// normalizeStringList converts a string, []string, or []any of strings to a
// string slice. The bool is false for any other type.
func normalizeStringList(v any) ([]string, bool) {
	switch t := v.(type) {
	case string:
		return []string{t}, true
	case []string:
		return slices.Clone(t), true
	case []any:
		out := make([]string, 0, len(t))
		for _, raw := range t {
			if s, ok := raw.(string); ok {
				out = append(out, s)
			}
		}
		return out, true
	default:
		return nil, false
	}
}

// scopeSeq yields the scopes carried by a scope claim value, splitting a
// space-separated string (OAuth 2.0 style) or iterating an array.
func scopeSeq(v any) iter.Seq[string] {
	return claimscope.Seq(v)
}
