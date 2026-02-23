// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/security/secrets"
)

func BenchmarkValidateKeyLength(b *testing.B) {
	key := "my.valid-key_1"
	b.ResetTimer()
	for b.Loop() {
		_ = secrets.ValidateKeyLength(key)
	}
}

func BenchmarkValidateSecretKey(b *testing.B) {
	key := "my.valid-key_1"
	b.ResetTimer()
	for b.Loop() {
		_ = secrets.ValidateSecretKey(key)
	}
}

func BenchmarkCreateLockKey(b *testing.B) {
	for b.Loop() {
		_ = secrets.CreateLockKey("provider", "key")
	}
}
