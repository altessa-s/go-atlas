// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt_test

import (
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/auth/selfjwt"
)

func BenchmarkMint(b *testing.B) {
	p := newProvider(b, selfjwt.AlgEdDSA)
	m := selfjwt.NewMinter(p, selfjwt.WithIssuer(testIssuer))
	req := selfjwt.MintRequest{Subject: testSubject, Scopes: []string{"files:read"}, TTL: time.Hour}
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		if _, err := m.Mint(ctx, req); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkVerify(b *testing.B) {
	p := newProvider(b, selfjwt.AlgEdDSA)
	m := selfjwt.NewMinter(p, selfjwt.WithIssuer(testIssuer))
	v := selfjwt.NewVerifier(p, selfjwt.WithIssuer(testIssuer))
	ctx := b.Context()

	res, err := m.Mint(ctx, selfjwt.MintRequest{Subject: testSubject, TTL: time.Hour})
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := v.Verify(ctx, res.Token); err != nil {
			b.Fatal(err)
		}
	}
}
