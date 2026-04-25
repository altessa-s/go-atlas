// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package hash_test

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/altessa-s/go-atlas/core/encoding/hash"
)

func BenchmarkSHA256HexString(b *testing.B) {
	s := strings.Repeat("a", 1024)

	b.Run("Core", func(b *testing.B) {
		for b.Loop() {
			hash.SHA256HexString(s)
		}
	})

	b.Run("StdCopy", func(b *testing.B) {
		for b.Loop() {
			sum := sha256.Sum256([]byte(s))
			_ = hex.EncodeToString(sum[:])
		}
	})
}
