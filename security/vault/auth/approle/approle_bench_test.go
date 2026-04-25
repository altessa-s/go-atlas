// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package approle_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/security/vault/auth/approle"
)

func BenchmarkNew(b *testing.B) {
	for b.Loop() {
		_ = approle.New("role-123", "secret-456")
	}
}

func BenchmarkNew_WithMountPath(b *testing.B) {
	for b.Loop() {
		_ = approle.New("role-123", "secret-456", approle.WithMountPath("/auth/approle"))
	}
}

func BenchmarkName(b *testing.B) {
	m := approle.New("role", "secret")
	b.ResetTimer()
	for b.Loop() {
		_ = m.Name()
	}
}
