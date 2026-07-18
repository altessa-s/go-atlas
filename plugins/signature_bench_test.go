// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugins

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func BenchmarkVerifySignature_Ed25519(b *testing.B) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(b, err)

	data := make([]byte, 1<<20) // 1 MB
	hash := sha256.Sum256(data)
	sig := ed25519.Sign(priv, hash[:])

	sigPath := filepath.Join(b.TempDir(), "bench.so.sig")
	require.NoError(b, os.WriteFile(sigPath, sig, 0o644))

	b.ResetTimer()
	for b.Loop() {
		_ = verifySignature(pub, data, sigPath)
	}
}

func BenchmarkVerifySignature_ECDSA(b *testing.B) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(b, err)
	pub := &priv.PublicKey

	data := make([]byte, 1<<20) // 1MB plugin
	hash := sha256.Sum256(data)
	sig, err := ecdsa.SignASN1(rand.Reader, priv, hash[:])
	require.NoError(b, err)

	sigPath := filepath.Join(b.TempDir(), "bench.so.sig")
	require.NoError(b, os.WriteFile(sigPath, sig, 0o644))

	b.ResetTimer()
	for b.Loop() {
		_ = verifySignature(pub, data, sigPath)
	}
}

func BenchmarkVerifySignature_RSA(b *testing.B) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(b, err)
	pub := &priv.PublicKey

	data := make([]byte, 1<<20) // 1MB plugin
	hash := sha256.Sum256(data)
	sig, err := rsa.SignPSS(rand.Reader, priv, crypto.SHA256, hash[:], nil)
	require.NoError(b, err)

	sigPath := filepath.Join(b.TempDir(), "bench.so.sig")
	require.NoError(b, os.WriteFile(sigPath, sig, 0o644))

	b.ResetTimer()
	for b.Loop() {
		_ = verifySignature(pub, data, sigPath)
	}
}

func BenchmarkManager_VerifyPluginSignature_Disabled(b *testing.B) {
	mgr := NewManager(WithSignatureDisabled())
	data := make([]byte, 1<<20) // 1MB plugin

	b.ResetTimer()
	for b.Loop() {
		_ = mgr.verifyPluginSignature("test.so", "/path/test.so", data, "abc123")
	}
}

func BenchmarkReadAndHashFile_1MB(b *testing.B) {
	benchmarkReadAndHashFile(b, 1<<20)
}

func BenchmarkReadAndHashFile_10MB(b *testing.B) {
	benchmarkReadAndHashFile(b, 10<<20)
}

func BenchmarkReadAndHashFile_100MB(b *testing.B) {
	benchmarkReadAndHashFile(b, 100<<20)
}

func benchmarkReadAndHashFile(b *testing.B, size int) {
	dir := b.TempDir()
	path := filepath.Join(dir, "plugin.so")

	data := make([]byte, size)
	_, err := rand.Read(data)
	require.NoError(b, err)
	require.NoError(b, os.WriteFile(path, data, 0o644))

	b.SetBytes(int64(size))
	b.ResetTimer()
	for b.Loop() {
		_, _, _ = readAndHashFileWithCache(path, nil)
	}
}

// Benchmark concurrent signature verification
func BenchmarkVerifySignature_Concurrent(b *testing.B) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(b, err)

	// Create multiple plugins with signatures
	numPlugins := 10
	plugins := make([]struct {
		data []byte
		path string
	}, numPlugins)

	dir := b.TempDir()
	for i := range numPlugins {
		data := make([]byte, 1<<20) // 1MB each
		hash := sha256.Sum256(data)
		sig := ed25519.Sign(priv, hash[:])

		sigPath := filepath.Join(dir, fmt.Sprintf("plugin%d.sig", i))
		require.NoError(b, os.WriteFile(sigPath, sig, 0o644))

		plugins[i].data = data
		plugins[i].path = sigPath[:len(sigPath)-4] // Remove .sig extension
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			p := plugins[i%numPlugins]
			_ = verifySignature(pub, p.data, p.path+".sig")
			i++
		}
	})
}
