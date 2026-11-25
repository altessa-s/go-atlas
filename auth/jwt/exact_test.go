// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package jwt_test

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/jwt"
)

func TestExactRegisteredDates(t *testing.T) {
	t.Parallel()
	ks := newKeysets(t)[0]
	for _, tc := range []struct {
		name, claim, literal string
		nanos                int64
		err                  error
	}{
		{"expiration before", "exp", "1750000000.25", 200000000, nil},
		{"expiration after", "exp", "1750000000.25", 300000000, jwt.ErrTokenExpired},
		{"nanosecond", "exp", "1750000001.000000001", 1000000000, nil},
		{"sub-nanosecond", "exp", "1750000001.0000000001", 1000000000, nil},
		{"expiration equal", "exp", "1750000001.000000001", 1000000001, jwt.ErrTokenExpired},
		{"not before early", "nbf", "1750000000.75", 250000000, jwt.ErrTokenInvalid},
		{"not before inclusive", "nbf", "1750000000.75", 750000000, nil},
		{"issued in future", "iat", "1.7500000000000000001e9", 0, jwt.ErrTokenInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			claims := jwt.Claims{"exp": json.Number("2e9"), tc.claim: json.Number(tc.literal),
				"custom": []any{map[string]any{"n": 42}}}
			v := jwt.NewVerifier(jwt.StaticKey(ks.verification), jwt.WithVerifierClock(fakeClock{time.Unix(1750000000, tc.nanos)}), jwt.WithLeeway(0), jwt.WithIssuedAt())
			raw, err := jwt.NewSigner().Sign(ks.signing, claims)
			require.NoError(t, err)
			got, _, err := v.VerifySignature(t.Context(), raw)
			require.NoError(t, err)
			require.Equal(t, json.Number(tc.literal), got[tc.claim])
			require.Equal(t, []any{map[string]any{"n": float64(42)}}, got["custom"])
			require.ErrorIs(t, v.ValidateClaims(got), tc.err)
			_, err = v.Verify(t.Context(), raw)
			require.ErrorIs(t, err, tc.err)
		})
	}
}

func TestInvalidRegisteredNumber(t *testing.T) {
	t.Parallel()
	v := jwt.NewVerifier(nil)
	for _, value := range []any{math.NaN(), math.Inf(1), float64(1 << 63), json.Number("NaN"), json.Number("Inf"),
		json.Number("9223372036854775807.0001"), json.Number("-9223372036854775808.1")} {
		require.ErrorIs(t, v.ValidateClaims(jwt.Claims{"exp": value}), jwt.ErrTokenInvalid)
	}
}

func TestExactTimeProjection(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		literal          string
		sec, floor, ceil int64
	}{
		{"1.7500000001234567891e9", 1750000000, 123456789, 123456790},
		{"1750000000.25", 1750000000, 250000000, 250000000},
		{"-0.0000000001", -1, 999999999, 1000000000},
	} {
		t.Run(tc.literal, func(t *testing.T) {
			t.Parallel()
			c := jwt.Claims{"exp": json.Number(tc.literal), "iat": json.Number(tc.literal)}
			require.Equal(t, time.Unix(tc.sec, tc.floor).UTC(), c.Time("exp"))
			require.Equal(t, c.Time("exp"), c.IssuedAt())
			require.Equal(t, time.Unix(tc.sec, tc.ceil).UTC(), c.Expiry())
		})
	}
}

func TestExactTypedDates(t *testing.T) {
	t.Parallel()
	ks := newKeysets(t)[0]
	v := jwt.NewVerifier(jwt.StaticKey(ks.verification), jwt.WithVerifierClock(fakeClock{time.Unix(1750000001, 0)}))
	signer := jwt.NewSigner()
	claims := jwt.Claims{"exp": json.Number("2e9"), "nbf": json.Number("1.75e9"), "iat": json.Number("1750000000.0")}
	for _, present := range []bool{true, false} {
		if !present {
			delete(claims, "iat")
		}
		raw, err := signer.Sign(ks.signing, claims)
		require.NoError(t, err)
		base, err := jwt.VerifyInto[jwt.ClaimsBase](t.Context(), v, raw)
		require.NoError(t, err)
		raw, err = jwt.SignStruct(signer, ks.signing, base)
		require.NoError(t, err)
		got, err := v.Verify(t.Context(), raw)
		require.NoError(t, err)
		require.Equal(t, json.Number("2000000000"), got["exp"])
		require.Equal(t, json.Number("1750000000"), got["nbf"])
		require.Equal(t, present, got.Has("iat"))
		if present {
			require.Equal(t, json.Number("1750000000"), got["iat"])
		}
	}
}

func TestSignStructLargeDate(t *testing.T) {
	t.Parallel()
	ks := newKeysets(t)[0]
	raw, err := jwt.SignStruct(jwt.NewSigner(), ks.signing, jwt.ClaimsBase{ExpiresAt: 9007199254740993})
	require.NoError(t, err)
	v := jwt.NewVerifier(jwt.StaticKey(ks.verification))
	got, err := v.Verify(t.Context(), raw)
	require.NoError(t, err)
	require.Equal(t, json.Number("9007199254740993"), got["exp"])
}

func TestVerifyIntoUnrepresentableDate(t *testing.T) {
	t.Parallel()
	ks := newKeysets(t)[0]
	for _, tc := range []struct {
		field, value string
		now          int64
	}{
		{"iat", "0", 1750000001}, {"exp", "0", -1}, {"nbf", "0", 1750000001},
		{"nbf", "1750000000.75", 1750000001}, {"exp", "1750000004.25", 1750000001},
		{"iat", "-9223372036854775809", 1750000001}, {"exp", "9223372036854775808", 1750000001},
	} {
		t.Run(tc.field+tc.value, func(t *testing.T) {
			t.Parallel()
			c := jwt.Claims{"exp": int64(2000000000)}
			c[tc.field] = json.Number(tc.value)
			raw, err := jwt.NewSigner().Sign(ks.signing, c)
			require.NoError(t, err)
			v := jwt.NewVerifier(jwt.StaticKey(ks.verification), jwt.WithVerifierClock(fakeClock{time.Unix(tc.now, 0)}), jwt.WithLeeway(0))
			_, err = jwt.VerifyInto[jwt.ClaimsBase](t.Context(), v, raw)
			require.ErrorIs(t, err, jwt.ErrTokenInvalid)
		})
	}
}

func TestExactLeeway(t *testing.T) {
	t.Parallel()
	ks := newKeysets(t)[0]
	now := time.Unix(1750000000, 250000000)
	v := jwt.NewVerifier(jwt.StaticKey(ks.verification), jwt.WithVerifierClock(fakeClock{now}), jwt.WithLeeway(time.Second))
	// The clock is still a tenth of a nanosecond inside the allowed skew.
	claims := jwt.Claims{"exp": json.Number("1749999999.2500000001")}
	raw, err := jwt.NewSigner().Sign(ks.signing, claims)
	require.NoError(t, err)
	_, err = v.Verify(t.Context(), raw)
	require.NoError(t, err)
	require.NoError(t, v.ValidateClaims(claims))
	claims["exp"] = json.Number("1749999999.25")
	require.ErrorIs(t, v.ValidateClaims(claims), jwt.ErrTokenExpired)
}

func TestDecodeRegisteredDateRange(t *testing.T) {
	t.Parallel()
	ks := newKeysets(t)[0]
	v := jwt.NewVerifier(jwt.StaticKey(ks.verification))
	for _, name := range []string{"exp", "nbf", "iat"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			raw, err := jwt.NewSigner().Sign(ks.signing, jwt.Claims{name: json.Number("9223372036854775807.0001")})
			require.NoError(t, err)
			_, _, err = v.VerifySignature(t.Context(), raw)
			require.ErrorIs(t, err, jwt.ErrTokenInvalid)
		})
	}
}

func TestGenericDecodeExactDate(t *testing.T) {
	t.Parallel()
	ks := newKeysets(t)[0]
	const literal = "1.7500000010000000001e9"
	raw, err := jwt.NewSigner().Sign(ks.signing, jwt.Claims{"exp": json.Number(literal)})
	require.NoError(t, err)
	v := jwt.NewVerifier(jwt.StaticKey(ks.verification), jwt.WithVerifierClock(fakeClock{time.Unix(1750000000, 0)}))
	claims, err := v.Verify(t.Context(), raw)
	require.NoError(t, err)
	var custom struct {
		Exp json.Number `json:"exp"`
	}
	require.NoError(t, claims.Decode(&custom))
	require.Equal(t, json.Number(literal), custom.Exp)
}
