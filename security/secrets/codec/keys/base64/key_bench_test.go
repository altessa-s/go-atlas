// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package base64_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/security/secrets/codec/keys/base64"
)

func BenchmarkKeyDecoder_Encode(b *testing.B) {
	decoder := base64.NewKeyDecoder()
	key := "my.secret-key_123"
	b.ResetTimer()
	for b.Loop() {
		_, _ = decoder.Encode(key)
	}
}

func BenchmarkKeyDecoder_Decode(b *testing.B) {
	decoder := base64.NewKeyDecoder()
	encodedKey, _ := decoder.Encode("my.secret-key_123")
	b.ResetTimer()
	for b.Loop() {
		_, _ = decoder.Decode(encodedKey)
	}
}
