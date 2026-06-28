// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package jwt_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/auth/jwt"
)

func BenchmarkSign(b *testing.B) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		b.Fatal(err)
	}
	_ = pub
	signer := jwt.NewSigner(jwt.WithIssuer("billing"))
	key := jwt.SigningKey{KeyID: "ed", Algorithm: jwt.AlgEdDSA, Key: priv}
	claims := jwt.Claims{"sub": "tenant-1", "exp": time.Now().Add(time.Hour).Unix()}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := signer.Sign(key, claims); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkVerify(b *testing.B) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		b.Fatal(err)
	}
	signer := jwt.NewSigner(jwt.WithIssuer("billing"))
	key := jwt.SigningKey{KeyID: "ed", Algorithm: jwt.AlgEdDSA, Key: priv}
	claims := jwt.Claims{"sub": "tenant-1", "iss": "billing", "exp": time.Now().Add(time.Hour).Unix()}
	raw, err := signer.Sign(key, claims)
	if err != nil {
		b.Fatal(err)
	}
	verifier := jwt.NewVerifier(jwt.StaticKey(jwt.VerificationKey{Algorithm: jwt.AlgEdDSA, Key: pub}), jwt.WithIssuer("billing"))

	b.ReportAllocs()
	for b.Loop() {
		if _, err := verifier.Verify(b.Context(), raw); err != nil {
			b.Fatal(err)
		}
	}
}
