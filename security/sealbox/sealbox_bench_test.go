// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sealbox_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/security/sealbox"
)

func BenchmarkSeal(b *testing.B) {
	box, err := sealbox.New(newKey(b))
	if err != nil {
		b.Fatal(err)
	}
	plaintext := []byte("sk-0123456789abcdef0123456789abcdef")
	aad := []byte("ai-provider:p1|openai")
	b.ReportAllocs()
	for b.Loop() {
		if _, err := box.Seal(plaintext, aad); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkOpen(b *testing.B) {
	box, err := sealbox.New(newKey(b), newKey(b))
	if err != nil {
		b.Fatal(err)
	}
	aad := []byte("ai-provider:p1|openai")
	sealed, err := box.Seal([]byte("sk-0123456789abcdef0123456789abcdef"), aad)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := box.Open(sealed, aad); err != nil {
			b.Fatal(err)
		}
	}
}
