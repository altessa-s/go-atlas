// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package jwt_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/jwt"
)

type tenantClaims struct {
	jwt.ClaimsBase
	Tenant int64 `json:"tenant"`
	Level  int   `json:"level"`
}

func TestVerifyIntoKeepsLargeIntegers(t *testing.T) {
	t.Parallel()

	ks := newKeysets(t)[0]
	signer := jwt.NewSigner()
	verifier := jwt.NewVerifier(jwt.StaticKey(ks.verification))

	for _, tenant := range []int64{9007199254740993, 9007199254740995, -9007199254740993, 1<<62 + 1} {
		base, err := signer.NewBase("sub", time.Hour)
		require.NoError(t, err)
		raw, err := jwt.SignStruct(signer, ks.signing, tenantClaims{ClaimsBase: base, Tenant: tenant, Level: 3})
		require.NoError(t, err)

		got, err := jwt.VerifyInto[tenantClaims](t.Context(), verifier, raw)
		require.NoError(t, err)
		require.Equal(t, tenant, got.Tenant)
		require.Equal(t, 3, got.Level)
	}
}

func TestVerifiedNumbersKeepTypeWhenExact(t *testing.T) {
	t.Parallel()

	ks := newKeysets(t)[0]
	claims := jwt.Claims{
		"exp":   json.Number("2000000000"),
		"level": json.Number("3"),
		"ratio": json.Number("0.1"),
		"big":   json.Number("9007199254740993"),
		"nested": map[string]any{
			"n": json.Number("42"), "big": json.Number("9007199254740993"),
		},
		"list": []any{json.Number("1"), json.Number("9007199254740993")},
	}
	raw, err := jwt.NewSigner().Sign(ks.signing, claims)
	require.NoError(t, err)

	got, _, err := jwt.NewVerifier(jwt.StaticKey(ks.verification)).VerifySignature(t.Context(), raw)
	require.NoError(t, err)

	level, ok := jwt.Get[float64](got, "level")
	require.True(t, ok, "small integers keep float64")
	require.InDelta(t, 3, level, 0)
	ratio, ok := jwt.Get[float64](got, "ratio")
	require.True(t, ok, "ordinary decimals keep float64")
	require.InDelta(t, 0.1, ratio, 0)

	big, ok := jwt.Get[json.Number](got, "big")
	require.True(t, ok, "inexact numbers keep their spelling")
	require.Equal(t, json.Number("9007199254740993"), big)

	nested := got["nested"].(map[string]any)
	require.InDelta(t, 42, nested["n"], 0)
	require.Equal(t, json.Number("9007199254740993"), nested["big"])
	list := got["list"].([]any)
	require.InDelta(t, 1, list[0], 0)
	require.Equal(t, json.Number("9007199254740993"), list[1])
}
