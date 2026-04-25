// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lockbox

import (
	"strings"
	"testing"
)

func BenchmarkValidateLockboxFolderId(b *testing.B) {
	id := "b1g2h3j4k5l6m7n8o9p0"
	for b.Loop() {
		_ = validateLockboxFolderId(id)
	}
}

func BenchmarkValidateLockboxKeyId(b *testing.B) {
	id := "aje1234567890abcdef"
	for b.Loop() {
		_ = validateLockboxKeyId(id)
	}
}

func BenchmarkValidateLockboxServiceAccountId(b *testing.B) {
	id := "aje1234567890abcdef"
	for b.Loop() {
		_ = validateLockboxServiceAccountId(id)
	}
}

func BenchmarkValidateLockboxSecretKey(b *testing.B) {
	key := "my-secret-key"
	for b.Loop() {
		_ = validateLockboxSecretKey(key)
	}
}

func BenchmarkValidateLockboxFolderId_Invalid(b *testing.B) {
	id := strings.Repeat("a", 51)
	for b.Loop() {
		_ = validateLockboxFolderId(id)
	}
}

func BenchmarkAddJitter(b *testing.B) {
	for b.Loop() {
		_ = addJitter(clientTokenLifetime, tokenLifetimeJitterPercent)
	}
}
