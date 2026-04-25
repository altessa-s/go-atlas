// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package userpass_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/security/vault/auth/userpass"
)

func BenchmarkNew(b *testing.B) {
	for b.Loop() {
		_ = userpass.New("admin", "password123")
	}
}

func BenchmarkNew_WithMountPath(b *testing.B) {
	for b.Loop() {
		_ = userpass.New("admin", "pass", userpass.WithMountPath("/auth/userpass"))
	}
}

func BenchmarkName(b *testing.B) {
	m := userpass.New("admin", "pass")
	b.ResetTimer()
	for b.Loop() {
		_ = m.Name()
	}
}
