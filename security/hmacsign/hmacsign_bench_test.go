// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package hmacsign_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/security/hmacsign"
)

func BenchmarkSign(b *testing.B) {
	signer := hmacsign.NewSigner(hmacsign.Stripe(), []byte("whsec_bench"))
	body := []byte(`{"event":"payment.succeeded","id":"evt_bench"}`)
	b.ReportAllocs()
	for b.Loop() {
		_ = signer.Sign(body)
	}
}

func BenchmarkVerify(b *testing.B) {
	secret := []byte("whsec_bench")
	body := []byte(`{"event":"payment.succeeded","id":"evt_bench"}`)
	signer := hmacsign.NewSigner(hmacsign.Stripe(), secret)
	verifier := hmacsign.NewVerifier(hmacsign.Stripe(), secret, hmacsign.WithTolerance(0))
	header := signer.Sign(body)
	b.ReportAllocs()
	for b.Loop() {
		_ = verifier.Verify(header, body)
	}
}
