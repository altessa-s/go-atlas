// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package jwt_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/jwt"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	gojwt "github.com/golang-jwt/jwt/v5"
)

// fakeClock is a pinned wall clock for deterministic temporal-claim tests.
type fakeClock struct{ t time.Time }

func (c fakeClock) Now() time.Time { return c.t }

// keyset bundles a signing key with the verification key that checks it.
type keyset struct {
	alg          jwt.Algorithm
	signing      jwt.SigningKey
	verification jwt.VerificationKey
}

// newKeysets returns one keyset per default asymmetric algorithm.
func newKeysets(tb testing.TB) []keyset {
	tb.Helper()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(tb, err)

	ec, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(tb, err)

	rsaKey := testhelpers.GenerateRSAKey(tb, 2048)

	return []keyset{
		{
			alg:          jwt.AlgEdDSA,
			signing:      jwt.SigningKey{KeyID: "ed", Algorithm: jwt.AlgEdDSA, Key: priv},
			verification: jwt.VerificationKey{Algorithm: jwt.AlgEdDSA, Key: pub},
		},
		{
			alg:          jwt.AlgES256,
			signing:      jwt.SigningKey{KeyID: "ec", Algorithm: jwt.AlgES256, Key: ec},
			verification: jwt.VerificationKey{Algorithm: jwt.AlgES256, Key: &ec.PublicKey},
		},
		{
			alg:          jwt.AlgRS256,
			signing:      jwt.SigningKey{KeyID: "rsa", Algorithm: jwt.AlgRS256, Key: rsaKey},
			verification: jwt.VerificationKey{Algorithm: jwt.AlgRS256, Key: &rsaKey.PublicKey},
		},
	}
}

func TestRoundTrip(t *testing.T) {
	t.Parallel()

	for _, ks := range newKeysets(t) {
		t.Run(ks.alg.String(), func(t *testing.T) {
			t.Parallel()

			signer := jwt.NewSigner(jwt.WithIssuer("billing"))
			claims, err := signer.NewClaims("tenant-1", time.Hour, "files:read", "files:write")
			require.NoError(t, err)

			raw, err := signer.Sign(ks.signing, claims)
			require.NoError(t, err)

			verifier := jwt.NewVerifier(jwt.StaticKey(ks.verification), jwt.WithIssuer("billing"))
			got, err := verifier.Verify(t.Context(), raw)
			require.NoError(t, err)

			require.Equal(t, "tenant-1", got.Subject())
			require.Equal(t, "billing", got.Issuer())
			require.NotEmpty(t, got.ID())
			require.Equal(t, []string{"files:read", "files:write"}, got.Scopes())
			require.False(t, got.Expiry().IsZero())
		})
	}
}

func TestVerifyTemporalClaims(t *testing.T) {
	t.Parallel()

	ks := newKeysets(t)[0]
	now := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	signer := jwt.NewSigner()

	tests := []struct {
		name    string
		exp     time.Time
		nbf     time.Time
		wantErr error
	}{
		{name: "valid", exp: now.Add(time.Hour)},
		{name: "expired", exp: now.Add(-time.Hour), wantErr: jwt.ErrTokenExpired},
		{name: "expired within leeway", exp: now.Add(-10 * time.Second)},
		{name: "not yet valid", exp: now.Add(time.Hour), nbf: now.Add(time.Hour), wantErr: jwt.ErrTokenInvalid},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			claims := jwt.Claims{"sub": "s", "exp": tc.exp.Unix()}
			if !tc.nbf.IsZero() {
				claims.Set("nbf", tc.nbf.Unix())
			}
			raw, err := signer.Sign(ks.signing, claims)
			require.NoError(t, err)

			verifier := jwt.NewVerifier(jwt.StaticKey(ks.verification), jwt.WithClock(fakeClock{now}))
			_, err = verifier.Verify(t.Context(), raw)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestVerifyExpirationRequired(t *testing.T) {
	t.Parallel()

	ks := newKeysets(t)[0]
	raw, err := jwt.NewSigner().Sign(ks.signing, jwt.Claims{"sub": "s"})
	require.NoError(t, err)

	verifier := jwt.NewVerifier(jwt.StaticKey(ks.verification))
	_, err = verifier.Verify(t.Context(), raw)
	require.ErrorIs(t, err, jwt.ErrTokenInvalid)
}

func TestVerifyIssuerAndAudience(t *testing.T) {
	t.Parallel()

	ks := newKeysets(t)[0]
	now := time.Now()
	signer := jwt.NewSigner()
	base := func() jwt.Claims {
		return jwt.Claims{"sub": "s", "exp": now.Add(time.Hour).Unix(), "iss": "billing", "aud": []any{"api", "web"}}
	}

	tests := []struct {
		name    string
		opts    []jwt.Option
		wantErr error
	}{
		{name: "issuer match", opts: []jwt.Option{jwt.WithIssuer("billing")}},
		{name: "issuer mismatch", opts: []jwt.Option{jwt.WithIssuer("other")}, wantErr: jwt.ErrTokenInvalid},
		{name: "audience match", opts: []jwt.Option{jwt.WithAudiences("web")}},
		{name: "audience mismatch", opts: []jwt.Option{jwt.WithAudiences("admin")}, wantErr: jwt.ErrTokenInvalid},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			raw, err := signer.Sign(ks.signing, base())
			require.NoError(t, err)

			verifier := jwt.NewVerifier(jwt.StaticKey(ks.verification), tc.opts...)
			_, err = verifier.Verify(t.Context(), raw)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestVerifyRequiredClaims(t *testing.T) {
	t.Parallel()

	ks := newKeysets(t)[0]
	raw, err := jwt.NewSigner().Sign(ks.signing, jwt.Claims{"sub": "s", "exp": time.Now().Add(time.Hour).Unix()})
	require.NoError(t, err)

	verifier := jwt.NewVerifier(jwt.StaticKey(ks.verification), jwt.WithRequiredClaims("tenant"))
	_, err = verifier.Verify(t.Context(), raw)
	require.ErrorIs(t, err, jwt.ErrClaimMissing)
}

func TestVerifyAlgorithmAllowList(t *testing.T) {
	t.Parallel()

	ks := newKeysets(t)[1] // ES256
	raw, err := jwt.NewSigner().Sign(ks.signing, jwt.Claims{"sub": "s", "exp": time.Now().Add(time.Hour).Unix()})
	require.NoError(t, err)

	// Verifier accepts only RS256, so an ES256 token is rejected before its
	// signature is checked.
	verifier := jwt.NewVerifier(jwt.StaticKey(ks.verification), jwt.WithAllowedAlgorithms(jwt.AlgRS256))
	_, err = verifier.Verify(t.Context(), raw)
	require.ErrorIs(t, err, jwt.ErrTokenInvalid)
}

func TestVerifyKeyAlgorithmBinding(t *testing.T) {
	t.Parallel()

	ks := newKeysets(t)[2] // RS256
	raw, err := jwt.NewSigner().Sign(ks.signing, jwt.Claims{"sub": "s", "exp": time.Now().Add(time.Hour).Unix()})
	require.NoError(t, err)

	// Resolver returns the right key but binds it to a different algorithm.
	resolver := jwt.StaticKey(jwt.VerificationKey{Algorithm: jwt.AlgES256, Key: ks.verification.Key})
	verifier := jwt.NewVerifier(resolver)
	_, err = verifier.Verify(t.Context(), raw)
	require.ErrorIs(t, err, jwt.ErrAlgorithmNotAllowed)
}

func TestVerifyRejectsHMACForgery(t *testing.T) {
	t.Parallel()

	ks := newKeysets(t)[2] // RS256 public key
	// Classic algorithm-confusion attack: forge an HS256 token, hoping the
	// verifier feeds the RSA public key to HMAC. The asymmetric-only allow-list
	// must reject it.
	forged := gojwt.NewWithClaims(gojwt.SigningMethodHS256, gojwt.MapClaims{
		"sub": "attacker",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	forged.Header["kid"] = "rsa"
	raw, err := forged.SignedString([]byte("secret"))
	require.NoError(t, err)

	verifier := jwt.NewVerifier(jwt.StaticKey(ks.verification))
	_, err = verifier.Verify(t.Context(), raw)
	require.ErrorIs(t, err, jwt.ErrTokenInvalid)
}

func TestVerifyResolverErrorPropagates(t *testing.T) {
	t.Parallel()

	ks := newKeysets(t)[0]
	raw, err := jwt.NewSigner().Sign(ks.signing, jwt.Claims{"sub": "s", "exp": time.Now().Add(time.Hour).Unix()})
	require.NoError(t, err)

	errSubjectUnknown := errors.New("subject unknown")
	resolver := jwt.KeyResolverFunc(func(context.Context, jwt.Header, jwt.Claims) (jwt.VerificationKey, error) {
		return jwt.VerificationKey{}, errSubjectUnknown
	})
	verifier := jwt.NewVerifier(resolver)
	_, err = verifier.Verify(t.Context(), raw)
	require.ErrorIs(t, err, errSubjectUnknown)
}

func TestResolverSeesHeaderAndClaims(t *testing.T) {
	t.Parallel()

	ks := newKeysets(t)[0]
	raw, err := jwt.NewSigner().Sign(ks.signing, jwt.Claims{"sub": "tenant-9", "exp": time.Now().Add(time.Hour).Unix()})
	require.NoError(t, err)

	var gotHdr jwt.Header
	var gotSub string
	resolver := jwt.KeyResolverFunc(func(_ context.Context, hdr jwt.Header, unverified jwt.Claims) (jwt.VerificationKey, error) {
		gotHdr = hdr
		gotSub = unverified.Subject()
		return ks.verification, nil
	})
	_, err = jwt.NewVerifier(resolver).Verify(t.Context(), raw)
	require.NoError(t, err)
	require.Equal(t, "ed", gotHdr.Kid)
	require.Equal(t, jwt.AlgEdDSA.String(), gotHdr.Alg)
	require.Equal(t, "tenant-9", gotSub)
}

func TestSignRejectsBadKey(t *testing.T) {
	t.Parallel()

	ks := newKeysets(t)[0]
	signer := jwt.NewSigner()

	t.Run("empty key id", func(t *testing.T) {
		t.Parallel()
		bad := ks.signing
		bad.KeyID = ""
		_, err := signer.Sign(bad, jwt.Claims{"sub": "s"})
		require.ErrorIs(t, err, jwt.ErrSigningKeyInvalid)
	})

	t.Run("disallowed algorithm", func(t *testing.T) {
		t.Parallel()
		bad := jwt.SigningKey{KeyID: "k", Algorithm: "HS256", Key: []byte("secret")}
		_, err := signer.Sign(bad, jwt.Claims{"sub": "s"})
		require.ErrorIs(t, err, jwt.ErrAlgorithmNotAllowed)
	})
}

func TestClaimsAccessors(t *testing.T) {
	t.Parallel()

	exp := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	c := jwt.Claims{
		"sub":   "tenant-1",
		"iss":   "billing",
		"jti":   "abc",
		"aud":   []any{"api", "web"},
		"scope": "files:read files:write",
		"exp":   float64(exp.Unix()),
	}

	require.Equal(t, "tenant-1", c.Subject())
	require.Equal(t, "billing", c.Issuer())
	require.Equal(t, "abc", c.ID())
	require.Equal(t, []string{"api", "web"}, c.Audience())
	require.Equal(t, []string{"files:read", "files:write"}, c.Scopes())
	require.Equal(t, exp, c.Expiry())
	require.True(t, c.Has("scope"))
	require.False(t, c.Has("missing"))

	t.Run("scope as array", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, []string{"a", "b"}, jwt.Claims{"scope": []any{"a", "b"}}.Scopes())
	})

	t.Run("audience as single string", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, []string{"only"}, jwt.Claims{"aud": "only"}.Audience())
	})
}

// headerTyp returns the typ header of raw without verifying its signature.
func headerTyp(tb testing.TB, raw string) string {
	tb.Helper()
	tok, _, err := gojwt.NewParser().ParseUnverified(raw, gojwt.MapClaims{})
	require.NoError(tb, err)
	typ, _ := tok.Header["typ"].(string)
	return typ
}

func TestTokenTypeHeader(t *testing.T) {
	t.Parallel()

	ks := newKeysets(t)[0]
	exp := time.Now().Add(time.Hour).Unix()

	t.Run("default typ is JWT", func(t *testing.T) {
		t.Parallel()
		raw, err := jwt.NewSigner().Sign(ks.signing, jwt.Claims{"sub": "s", "exp": exp})
		require.NoError(t, err)
		require.Equal(t, jwt.TypeJWT, headerTyp(t, raw))
	})

	t.Run("WithTokenType sets at+jwt", func(t *testing.T) {
		t.Parallel()
		raw, err := jwt.NewSigner(jwt.WithTokenType(jwt.TypeAccessToken)).Sign(ks.signing, jwt.Claims{"sub": "s", "exp": exp})
		require.NoError(t, err)
		require.Equal(t, jwt.TypeAccessToken, headerTyp(t, raw))
	})
}

func TestVerifyExpectedTokenType(t *testing.T) {
	t.Parallel()

	ks := newKeysets(t)[0]
	exp := time.Now().Add(time.Hour).Unix()
	accessToken, err := jwt.NewSigner(jwt.WithTokenType(jwt.TypeAccessToken)).Sign(ks.signing, jwt.Claims{"sub": "s", "exp": exp})
	require.NoError(t, err)
	idToken, err := jwt.NewSigner().Sign(ks.signing, jwt.Claims{"sub": "s", "exp": exp})
	require.NoError(t, err)

	t.Run("accepts matching type", func(t *testing.T) {
		t.Parallel()
		v := jwt.NewVerifier(jwt.StaticKey(ks.verification), jwt.WithExpectedTokenType(jwt.TypeAccessToken))
		_, err := v.Verify(t.Context(), accessToken)
		require.NoError(t, err)
	})

	t.Run("rejects wrong type", func(t *testing.T) {
		t.Parallel()
		v := jwt.NewVerifier(jwt.StaticKey(ks.verification), jwt.WithExpectedTokenType(jwt.TypeAccessToken))
		_, err := v.Verify(t.Context(), idToken)
		require.ErrorIs(t, err, jwt.ErrTokenTypeInvalid)
	})

	t.Run("unset leaves type unchecked", func(t *testing.T) {
		t.Parallel()
		v := jwt.NewVerifier(jwt.StaticKey(ks.verification))
		_, err := v.Verify(t.Context(), accessToken)
		require.NoError(t, err)
	})

	t.Run("match is case- and prefix-insensitive", func(t *testing.T) {
		t.Parallel()
		raw, err := jwt.NewSigner(jwt.WithTokenType("AT+JWT")).Sign(ks.signing, jwt.Claims{"sub": "s", "exp": exp})
		require.NoError(t, err)
		v := jwt.NewVerifier(jwt.StaticKey(ks.verification), jwt.WithExpectedTokenType("application/at+jwt"))
		_, err = v.Verify(t.Context(), raw)
		require.NoError(t, err)
	})
}

func TestClaimsGet(t *testing.T) {
	t.Parallel()

	c := jwt.Claims{"role": "admin", "level": float64(5), "active": true}

	role, ok := jwt.Get[string](c, "role")
	require.True(t, ok)
	require.Equal(t, "admin", role)

	level, ok := jwt.Get[float64](c, "level")
	require.True(t, ok)
	require.Equal(t, float64(5), level)

	active, ok := jwt.Get[bool](c, "active")
	require.True(t, ok)
	require.True(t, active)

	_, ok = jwt.Get[string](c, "missing")
	require.False(t, ok)

	// JSON numbers decode as float64, so an int assertion does not match.
	_, ok = jwt.Get[int](c, "level")
	require.False(t, ok)
}

func TestClaimsDecode(t *testing.T) {
	t.Parallel()

	type myClaims struct {
		Subject string   `json:"sub"`
		Role    string   `json:"role"`
		Level   int      `json:"level"`
		Groups  []string `json:"groups"`
	}
	c := jwt.Claims{"sub": "tenant-1", "role": "admin", "level": float64(5), "groups": []any{"a", "b"}}

	var mc myClaims
	require.NoError(t, c.Decode(&mc))
	require.Equal(t, myClaims{Subject: "tenant-1", Role: "admin", Level: 5, Groups: []string{"a", "b"}}, mc)
}

// Custom claims survive a Sign -> Verify round trip and read back via Get.
func TestCustomClaimsRoundTrip(t *testing.T) {
	t.Parallel()

	ks := newKeysets(t)[0]
	raw, err := jwt.NewSigner().Sign(ks.signing, jwt.Claims{
		"sub":  "tenant-1",
		"exp":  time.Now().Add(time.Hour).Unix(),
		"role": "admin",
	})
	require.NoError(t, err)

	got, err := jwt.NewVerifier(jwt.StaticKey(ks.verification)).Verify(t.Context(), raw)
	require.NoError(t, err)
	role, ok := jwt.Get[string](got, "role")
	require.True(t, ok)
	require.Equal(t, "admin", role)
}

// typedClaims is a service-defined claim struct embedding jwt.ClaimsBase to use
// the typed Sign/Verify sugar.
type typedClaims struct {
	jwt.ClaimsBase
	Role   string   `json:"role"`
	Groups []string `json:"groups"`
}

func TestSignStructVerifyIntoRoundTrip(t *testing.T) {
	t.Parallel()

	for _, ks := range newKeysets(t) {
		t.Run(ks.alg.String(), func(t *testing.T) {
			t.Parallel()

			signer := jwt.NewSigner(jwt.WithIssuer("billing"))
			base, err := signer.NewBase("tenant-1", time.Hour)
			require.NoError(t, err)
			base.Audience = jwt.Audience{"api", "web"}

			raw, err := jwt.SignStruct(signer, ks.signing, typedClaims{
				ClaimsBase: base,
				Role:       "admin",
				Groups:     []string{"eng", "ops"},
			})
			require.NoError(t, err)

			verifier := jwt.NewVerifier(jwt.StaticKey(ks.verification), jwt.WithIssuer("billing"), jwt.WithAudiences("web"))
			got, err := jwt.VerifyInto[typedClaims](t.Context(), verifier, raw)
			require.NoError(t, err)

			require.Equal(t, "tenant-1", got.Subject)
			require.Equal(t, "billing", got.Issuer)
			require.NotEmpty(t, got.ID)
			require.Equal(t, jwt.Audience{"api", "web"}, got.Audience)
			require.False(t, got.ExpiryTime().IsZero())
			require.Equal(t, "admin", got.Role)
			require.Equal(t, []string{"eng", "ops"}, got.Groups)
		})
	}
}

func TestAudienceUnmarshal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want jwt.Audience
	}{
		{name: "single string", raw: `"api"`, want: jwt.Audience{"api"}},
		{name: "array", raw: `["api","web"]`, want: jwt.Audience{"api", "web"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got jwt.Audience
			require.NoError(t, json.Unmarshal([]byte(tc.raw), &got))
			require.Equal(t, tc.want, got)

			// MarshalJSON always emits an array; it round-trips back to the same value.
			data, err := json.Marshal(got)
			require.NoError(t, err)
			var back jwt.Audience
			require.NoError(t, json.Unmarshal(data, &back))
			require.Equal(t, tc.want, back)
		})
	}
}

// VerifyInto runs the full verifier pipeline, so its checks reject typed tokens
// just as Verify rejects map tokens.
func TestVerifyIntoInheritsChecks(t *testing.T) {
	t.Parallel()

	ks := newKeysets(t)[0]
	signer := jwt.NewSigner(jwt.WithIssuer("billing"), jwt.WithTokenType(jwt.TypeAccessToken))
	base, err := signer.NewBase("tenant-1", time.Hour)
	require.NoError(t, err)
	raw, err := jwt.SignStruct(signer, ks.signing, typedClaims{ClaimsBase: base, Role: "admin"})
	require.NoError(t, err)

	t.Run("issuer mismatch", func(t *testing.T) {
		t.Parallel()
		v := jwt.NewVerifier(jwt.StaticKey(ks.verification), jwt.WithIssuer("other"))
		_, err := jwt.VerifyInto[typedClaims](t.Context(), v, raw)
		require.ErrorIs(t, err, jwt.ErrTokenInvalid)
	})

	t.Run("token type mismatch", func(t *testing.T) {
		t.Parallel()
		v := jwt.NewVerifier(jwt.StaticKey(ks.verification), jwt.WithIssuer("billing"), jwt.WithExpectedTokenType("id+jwt"))
		_, err := jwt.VerifyInto[typedClaims](t.Context(), v, raw)
		require.ErrorIs(t, err, jwt.ErrTokenTypeInvalid)
	})
}

func TestNewBase(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	signer := jwt.NewSigner(
		jwt.WithIssuer("billing"),
		jwt.WithClock(fakeClock{now}),
		jwt.WithMaxTokenLifetime(2*time.Hour),
	)

	t.Run("fills registered claims", func(t *testing.T) {
		t.Parallel()
		base, err := signer.NewBase("tenant-1", time.Hour)
		require.NoError(t, err)
		require.Equal(t, "billing", base.Issuer)
		require.Equal(t, "tenant-1", base.Subject)
		require.NotEmpty(t, base.ID)
		require.Equal(t, now, base.IssuedAtTime())
		require.Equal(t, now, base.NotBeforeTime())
		require.Equal(t, now.Add(time.Hour), base.ExpiryTime())
	})

	t.Run("clamps oversized ttl", func(t *testing.T) {
		t.Parallel()
		base, err := signer.NewBase("tenant-1", 100*time.Hour)
		require.NoError(t, err)
		require.Equal(t, now.Add(2*time.Hour), base.ExpiryTime())
	})

	t.Run("clamps non-positive ttl", func(t *testing.T) {
		t.Parallel()
		base, err := signer.NewBase("tenant-1", 0)
		require.NoError(t, err)
		require.Equal(t, now.Add(2*time.Hour), base.ExpiryTime())
	})
}

func TestVerifyWithSubject(t *testing.T) {
	t.Parallel()

	ks := newKeysets(t)[0]
	exp := time.Now().Add(time.Hour).Unix()
	raw, err := jwt.NewSigner().Sign(ks.signing, jwt.Claims{"sub": "svc-1", "exp": exp})
	require.NoError(t, err)

	t.Run("match", func(t *testing.T) {
		t.Parallel()
		v := jwt.NewVerifier(jwt.StaticKey(ks.verification), jwt.WithSubject("svc-1"))
		got, err := v.Verify(t.Context(), raw)
		require.NoError(t, err)
		require.Equal(t, "svc-1", got.Subject())
	})

	t.Run("mismatch", func(t *testing.T) {
		t.Parallel()
		v := jwt.NewVerifier(jwt.StaticKey(ks.verification), jwt.WithSubject("other"))
		_, err := v.Verify(t.Context(), raw)
		require.ErrorIs(t, err, jwt.ErrTokenInvalid)
	})
}

func TestVerifyIssuedAt(t *testing.T) {
	t.Parallel()

	ks := newKeysets(t)[0]
	now := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	v := jwt.NewVerifier(jwt.StaticKey(ks.verification), jwt.WithClock(fakeClock{now}), jwt.WithIssuedAt())

	future, err := jwt.NewSigner().Sign(ks.signing, jwt.Claims{
		"sub": "s", "exp": now.Add(time.Hour).Unix(), "iat": now.Add(time.Hour).Unix(),
	})
	require.NoError(t, err)
	_, err = v.Verify(t.Context(), future)
	require.ErrorIs(t, err, jwt.ErrTokenInvalid)

	past, err := jwt.NewSigner().Sign(ks.signing, jwt.Claims{
		"sub": "s", "exp": now.Add(time.Hour).Unix(), "iat": now.Add(-time.Minute).Unix(),
	})
	require.NoError(t, err)
	_, err = v.Verify(t.Context(), past)
	require.NoError(t, err)
}

func TestVerifyNotBeforeRequired(t *testing.T) {
	t.Parallel()

	ks := newKeysets(t)[0]
	exp := time.Now().Add(time.Hour).Unix()
	v := jwt.NewVerifier(jwt.StaticKey(ks.verification), jwt.WithNotBeforeRequired())

	noNbf, err := jwt.NewSigner().Sign(ks.signing, jwt.Claims{"sub": "s", "exp": exp})
	require.NoError(t, err)
	_, err = v.Verify(t.Context(), noNbf)
	require.ErrorIs(t, err, jwt.ErrTokenInvalid)

	withNbf, err := jwt.NewSigner().Sign(ks.signing, jwt.Claims{
		"sub": "s", "exp": exp, "nbf": time.Now().Add(-time.Minute).Unix(),
	})
	require.NoError(t, err)
	_, err = v.Verify(t.Context(), withNbf)
	require.NoError(t, err)
}

func TestVerifyWithHeader(t *testing.T) {
	t.Parallel()

	ks := newKeysets(t)[0]
	raw, err := jwt.NewSigner(jwt.WithTokenType(jwt.TypeAccessToken)).Sign(ks.signing, jwt.Claims{
		"sub": "s", "exp": time.Now().Add(time.Hour).Unix(),
	})
	require.NoError(t, err)

	_, hdr, err := jwt.NewVerifier(jwt.StaticKey(ks.verification)).VerifyWithHeader(t.Context(), raw)
	require.NoError(t, err)
	require.Equal(t, "ed", hdr.Kid)
	require.Equal(t, jwt.AlgEdDSA.String(), hdr.Alg)
	require.Equal(t, jwt.TypeAccessToken, hdr.Typ)

	// On a validation failure the returned header is the zero value.
	_, hdr, err = jwt.NewVerifier(jwt.StaticKey(ks.verification), jwt.WithIssuer("required")).VerifyWithHeader(t.Context(), raw)
	require.ErrorIs(t, err, jwt.ErrTokenInvalid)
	require.Equal(t, jwt.Header{}, hdr)
}

func TestVerifySignature(t *testing.T) {
	t.Parallel()

	ks := newKeysets(t)[0]

	t.Run("skips claim validation", func(t *testing.T) {
		t.Parallel()
		// Expired and wrong issuer: VerifySignature ignores both, returning the
		// claims and header for the caller to validate later.
		raw, err := jwt.NewSigner().Sign(ks.signing, jwt.Claims{
			"sub": "s", "exp": time.Now().Add(-time.Hour).Unix(), "iss": "whoever",
		})
		require.NoError(t, err)
		v := jwt.NewVerifier(jwt.StaticKey(ks.verification), jwt.WithIssuer("expected"))
		claims, hdr, err := v.VerifySignature(t.Context(), raw)
		require.NoError(t, err)
		require.Equal(t, "s", claims.Subject())
		require.Equal(t, "ed", hdr.Kid)
	})

	t.Run("rejects HMAC forgery", func(t *testing.T) {
		t.Parallel()
		forged := gojwt.NewWithClaims(gojwt.SigningMethodHS256, gojwt.MapClaims{"sub": "attacker"})
		forged.Header["kid"] = "ed"
		raw, err := forged.SignedString([]byte("secret"))
		require.NoError(t, err)
		_, _, err = jwt.NewVerifier(jwt.StaticKey(ks.verification)).VerifySignature(t.Context(), raw)
		require.ErrorIs(t, err, jwt.ErrTokenInvalid)
	})
}

func TestValidateClaims(t *testing.T) {
	t.Parallel()

	ks := newKeysets(t)[0]
	v := jwt.NewVerifier(jwt.StaticKey(ks.verification),
		jwt.WithIssuer("billing"), jwt.WithSubject("s"), jwt.WithRequiredClaims("tenant"))
	future := float64(time.Now().Add(time.Hour).Unix())

	t.Run("clean", func(t *testing.T) {
		t.Parallel()
		err := v.ValidateClaims(jwt.Claims{"iss": "billing", "sub": "s", "exp": future, "tenant": "t1"})
		require.NoError(t, err)
	})

	t.Run("issuer mismatch", func(t *testing.T) {
		t.Parallel()
		err := v.ValidateClaims(jwt.Claims{"iss": "other", "sub": "s", "exp": future, "tenant": "t1"})
		require.ErrorIs(t, err, jwt.ErrTokenInvalid)
	})

	t.Run("expired", func(t *testing.T) {
		t.Parallel()
		past := float64(time.Now().Add(-time.Hour).Unix())
		err := v.ValidateClaims(jwt.Claims{"iss": "billing", "sub": "s", "exp": past, "tenant": "t1"})
		require.ErrorIs(t, err, jwt.ErrTokenExpired)
	})

	t.Run("missing required claim", func(t *testing.T) {
		t.Parallel()
		err := v.ValidateClaims(jwt.Claims{"iss": "billing", "sub": "s", "exp": future})
		require.ErrorIs(t, err, jwt.ErrClaimMissing)
	})
}

func TestVerifyExpirationOptional(t *testing.T) {
	t.Parallel()

	ks := newKeysets(t)[0]
	noExp, err := jwt.NewSigner().Sign(ks.signing, jwt.Claims{"sub": "s"})
	require.NoError(t, err)

	// Default: exp is mandatory.
	_, err = jwt.NewVerifier(jwt.StaticKey(ks.verification)).Verify(t.Context(), noExp)
	require.ErrorIs(t, err, jwt.ErrTokenInvalid)

	// Optional: a token without exp is accepted.
	v := jwt.NewVerifier(jwt.StaticKey(ks.verification), jwt.WithExpirationOptional())
	_, err = v.Verify(t.Context(), noExp)
	require.NoError(t, err)

	// Optional still validates exp when present: an expired token is rejected.
	expired, err := jwt.NewSigner().Sign(ks.signing, jwt.Claims{"sub": "s", "exp": time.Now().Add(-time.Hour).Unix()})
	require.NoError(t, err)
	_, err = v.Verify(t.Context(), expired)
	require.ErrorIs(t, err, jwt.ErrTokenExpired)
}

func TestWithLeewayZero(t *testing.T) {
	t.Parallel()

	ks := newKeysets(t)[0]
	now := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	// Token expired 10s ago — within the 30s DefaultLeeway, outside zero leeway.
	raw, err := jwt.NewSigner().Sign(ks.signing, jwt.Claims{"sub": "s", "exp": now.Add(-10 * time.Second).Unix()})
	require.NoError(t, err)

	// Default leeway (30s) tolerates the 10s skew.
	_, err = jwt.NewVerifier(jwt.StaticKey(ks.verification), jwt.WithClock(fakeClock{now})).Verify(t.Context(), raw)
	require.NoError(t, err)

	// WithLeeway(0) enforces strict expiry, rejecting the same token.
	_, err = jwt.NewVerifier(jwt.StaticKey(ks.verification), jwt.WithClock(fakeClock{now}), jwt.WithLeeway(0)).Verify(t.Context(), raw)
	require.ErrorIs(t, err, jwt.ErrTokenExpired)
}
