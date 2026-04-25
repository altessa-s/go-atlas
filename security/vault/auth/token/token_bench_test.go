// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package token_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/security/vault/auth/token"
)

func BenchmarkNew(b *testing.B) {
	for b.Loop() {
		_ = token.New("hvs.CAESIJ1234567890abcdef")
	}
}

func BenchmarkName(b *testing.B) {
	m := token.New("hvs.abc")
	b.ResetTimer()
	for b.Loop() {
		_ = m.Name()
	}
}

func BenchmarkShutdown(b *testing.B) {
	m := token.New("hvs.abc")
	b.ResetTimer()
	for b.Loop() {
		_ = m.Shutdown()
	}
}
