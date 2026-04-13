// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugins

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func BenchmarkVerifySignature_Ed25519(b *testing.B) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(b, err)

	data := make([]byte, 1<<20) // 1 MB
	sig := ed25519.Sign(priv, data)

	sigPath := filepath.Join(b.TempDir(), "bench.so.sig")
	require.NoError(b, os.WriteFile(sigPath, sig, 0o644))

	b.ResetTimer()
	for b.Loop() {
		_ = verifySignature(pub, data, sigPath)
	}
}
