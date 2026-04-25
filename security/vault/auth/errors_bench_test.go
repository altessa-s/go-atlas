// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth_test

import (
	"errors"
	"testing"

	"github.com/altessa-s/go-atlas/security/vault/auth"
)

func BenchmarkIsAuthenticationError(b *testing.B) {
	err := auth.NewAuthError("test", "unauthorized", 401, nil)
	b.ResetTimer()
	for b.Loop() {
		_ = auth.IsAuthenticationError(err)
	}
}

func BenchmarkIsRetryableError(b *testing.B) {
	err := errors.New("generic error")
	b.ResetTimer()
	for b.Loop() {
		_ = auth.IsRetryableError(err)
	}
}
