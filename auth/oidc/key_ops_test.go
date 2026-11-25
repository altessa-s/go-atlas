// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func TestJWKSKeyOperationsAdmission(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"initial", "refresh"} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			var keys []map[string]any
			signers := make(map[string]*rsa.PrivateKey)
			for kid, ops := range map[string]any{"encrypt": []string{"encrypt"}, "empty": []string{}, "null": nil, "verify": []string{"verify"}} {
				key := testhelpers.GenerateRSAKey(t, 2048)
				jwk := rsaJWK(key, kid)
				delete(jwk, "use")
				jwk["key_ops"] = ops
				keys = append(keys, jwk)
				signers[kid] = key
			}
			body, err := json.Marshal(map[string]any{"keys": keys})
			require.NoError(t, err)
			initial := string(body)
			if phase == "refresh" {
				initial = jwksDoc(t, 1, 0)
			}
			endpoint, set := newJWKSBodyIdP(t, initial)
			p, err := newRedirectTestProvider(t, endpoint)
			require.NoError(t, err)
			if phase == "refresh" {
				set(string(body))
				require.NoError(t, p.RefreshJWKS(t.Context()))
			}
			for kid, key := range signers {
				for _, named := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/named=%t", kid, named), func(t *testing.T) {
						t.Parallel()
						token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{"iss": p.discovery.info.Issuer, "sub": "example"})
						if named {
							token.Header["kid"] = kid
						}
						signed, err := token.SignedString(key)
						require.NoError(t, err)
						_, err = p.userInfo.verifySignature(t.Context(), signed)
						if kid == "verify" {
							require.NoError(t, err)
						} else {
							require.Error(t, err)
						}
					})
				}
			}
		})
	}
}
