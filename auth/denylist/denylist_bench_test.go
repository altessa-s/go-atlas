// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package denylist_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/auth/denylist"
)

func BenchmarkIsRevoked(b *testing.B) {
	dl := denylist.New()
	dl.Revoke("revoked")
	b.ResetTimer()
	for b.Loop() {
		_ = dl.IsRevoked("revoked")
	}
}

// BenchmarkRevokeUntilLarge measures a write into a denylist that already holds
// many live entries — the mass-revocation path.
func BenchmarkRevokeUntilLarge(b *testing.B) {
	dl := denylist.New()
	expiry := time.Now().Add(time.Hour)
	for i := range 100_000 {
		dl.RevokeUntil(strconv.Itoa(i), expiry)
	}
	b.ReportAllocs()
	for b.Loop() {
		dl.RevokeUntil("hot", expiry)
	}
}
