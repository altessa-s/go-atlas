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
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMultiKeySignature_SingleKey(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Generate key
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	// Create and sign plugin
	pluginPath := filepath.Join(dir, "test.so")
	pluginData := []byte("test plugin")
	require.NoError(t, os.WriteFile(pluginPath, pluginData, 0o644))

	hash := sha256.Sum256(pluginData)
	sig := ed25519.Sign(priv, hash[:])
	require.NoError(t, os.WriteFile(pluginPath+".sig", sig, 0o644))

	// Test with single key in multi-key config
	mgr := NewManager(
		WithDir(dir),
		WithMultiKeySignature(MultiKeySignatureOptions{
			Mode:       SignatureRequire,
			PublicKeys: []crypto.PublicKey{pub},
		}),
	)

	// Should verify successfully
	err = mgr.verifyPluginSignature("test.so", pluginPath, pluginData, "abc123")
	require.NoError(t, err)
}

func TestMultiKeySignature_MultipleKeys_FirstMatches(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Generate two keys
	pub1, priv1, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	pub2, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	// Create and sign plugin with first key
	pluginPath := filepath.Join(dir, "test.so")
	pluginData := []byte("test plugin")
	require.NoError(t, os.WriteFile(pluginPath, pluginData, 0o644))

	hash := sha256.Sum256(pluginData)
	sig := ed25519.Sign(priv1, hash[:])
	require.NoError(t, os.WriteFile(pluginPath+".sig", sig, 0o644))

	// Test with multiple keys
	mgr := NewManager(
		WithDir(dir),
		WithMultiKeySignature(MultiKeySignatureOptions{
			Mode:       SignatureRequire,
			PublicKeys: []crypto.PublicKey{pub1, pub2}, // First key matches
		}),
	)

	// Should verify successfully with first key
	err = mgr.verifyPluginSignature("test.so", pluginPath, pluginData, "abc123")
	require.NoError(t, err)
}

func TestMultiKeySignature_MultipleKeys_SecondMatches(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Generate two keys
	pub1, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	pub2, priv2, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	// Create and sign plugin with second key
	pluginPath := filepath.Join(dir, "test.so")
	pluginData := []byte("test plugin")
	require.NoError(t, os.WriteFile(pluginPath, pluginData, 0o644))

	hash := sha256.Sum256(pluginData)
	sig := ed25519.Sign(priv2, hash[:])
	require.NoError(t, os.WriteFile(pluginPath+".sig", sig, 0o644))

	// Test with multiple keys
	mgr := NewManager(
		WithDir(dir),
		WithMultiKeySignature(MultiKeySignatureOptions{
			Mode:       SignatureRequire,
			PublicKeys: []crypto.PublicKey{pub1, pub2}, // Second key matches
		}),
	)

	// Should verify successfully with second key
	err = mgr.verifyPluginSignature("test.so", pluginPath, pluginData, "abc123")
	require.NoError(t, err)
}

func TestMultiKeySignature_NoKeysMatch(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Generate three keys
	pub1, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	pub2, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	_, priv3, err := ed25519.GenerateKey(rand.Reader) // Sign with different key
	require.NoError(t, err)

	// Create and sign plugin with third key (not in the list)
	pluginPath := filepath.Join(dir, "test.so")
	pluginData := []byte("test plugin")
	require.NoError(t, os.WriteFile(pluginPath, pluginData, 0o644))

	hash := sha256.Sum256(pluginData)
	sig := ed25519.Sign(priv3, hash[:])
	require.NoError(t, os.WriteFile(pluginPath+".sig", sig, 0o644))

	// Test with keys that don't match
	mgr := NewManager(
		WithDir(dir),
		WithMultiKeySignature(MultiKeySignatureOptions{
			Mode:       SignatureRequire,
			PublicKeys: []crypto.PublicKey{pub1, pub2}, // Neither key matches
		}),
	)

	// Should fail verification
	err = mgr.verifyPluginSignature("test.so", pluginPath, pluginData, "abc123")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrSignatureInvalid)
}

func TestMultiKeySignature_MixedAlgorithms(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Generate keys with different algorithms
	edPub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	ecPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	ecPub := &ecPriv.PublicKey

	rsaPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	rsaPub := &rsaPriv.PublicKey

	// Create and sign plugin with ECDSA
	pluginPath := filepath.Join(dir, "test.so")
	pluginData := []byte("test plugin")
	require.NoError(t, os.WriteFile(pluginPath, pluginData, 0o644))

	hash := sha256.Sum256(pluginData)
	sig, err := ecdsa.SignASN1(rand.Reader, ecPriv, hash[:])
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(pluginPath+".sig", sig, 0o644))

	// Test with mixed algorithm keys
	mgr := NewManager(
		WithDir(dir),
		WithMultiKeySignature(MultiKeySignatureOptions{
			Mode:       SignatureRequire,
			PublicKeys: []crypto.PublicKey{edPub, ecPub, rsaPub}, // ECDSA key matches
		}),
	)

	// Should verify successfully with ECDSA key
	err = mgr.verifyPluginSignature("test.so", pluginPath, pluginData, "abc123")
	require.NoError(t, err)
}

func TestMultiKeySignature_LoadFromFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Generate two keys and write to PEM files
	pub1, priv1, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	pub2, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	// Write public keys to PEM files
	key1Path := filepath.Join(dir, "key1.pem")
	key2Path := filepath.Join(dir, "key2.pem")

	writePubKeyPEM(t, key1Path, pub1)
	writePubKeyPEM(t, key2Path, pub2)

	// Create and sign plugin with first key
	pluginPath := filepath.Join(dir, "test.so")
	pluginData := []byte("test plugin")
	require.NoError(t, os.WriteFile(pluginPath, pluginData, 0o644))

	hash := sha256.Sum256(pluginData)
	sig := ed25519.Sign(priv1, hash[:])
	require.NoError(t, os.WriteFile(pluginPath+".sig", sig, 0o644))

	// Test loading keys from files
	mgr := NewManager(
		WithDir(dir),
		WithMultiKeySignature(MultiKeySignatureOptions{
			Mode:           SignatureRequire,
			PublicKeyPaths: []string{key1Path, key2Path},
		}),
	)

	// Should verify successfully
	err = mgr.verifyPluginSignature("test.so", pluginPath, pluginData, "abc123")
	require.NoError(t, err)
}

func TestMultiKeySignature_PartialKeyLoadFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Generate key and write to PEM file
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	validKeyPath := filepath.Join(dir, "valid.pem")
	writePubKeyPEM(t, validKeyPath, pub)

	// Invalid key path
	invalidKeyPath := filepath.Join(dir, "nonexistent.pem")

	// Create and sign plugin
	pluginPath := filepath.Join(dir, "test.so")
	pluginData := []byte("test plugin")
	require.NoError(t, os.WriteFile(pluginPath, pluginData, 0o644))

	hash := sha256.Sum256(pluginData)
	sig := ed25519.Sign(priv, hash[:])
	require.NoError(t, os.WriteFile(pluginPath+".sig", sig, 0o644))

	// Test with one valid and one invalid key path
	// Should still work with the valid key
	mgr := NewManager(
		WithDir(dir),
		WithMultiKeySignature(MultiKeySignatureOptions{
			Mode:           SignatureRequire,
			PublicKeyPaths: []string{invalidKeyPath, validKeyPath}, // One invalid, one valid
		}),
	)

	// Should verify successfully with the valid key
	err = mgr.verifyPluginSignature("test.so", pluginPath, pluginData, "abc123")
	require.NoError(t, err)
}

func TestMultiKeySignature_AllKeysLoadFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// All invalid key paths
	invalidPaths := []string{
		filepath.Join(dir, "nonexistent1.pem"),
		filepath.Join(dir, "nonexistent2.pem"),
	}

	// Test with all invalid key paths
	mgr := NewManager(
		WithDir(dir),
		WithMultiKeySignature(MultiKeySignatureOptions{
			Mode:           SignatureRequire,
			PublicKeyPaths: invalidPaths,
		}),
		WithSignatureDisabled(), // Override to avoid error during NewManager
	)

	// The signatureErr should be set
	require.NotNil(t, mgr.opts.signatureErr)
	require.ErrorIs(t, mgr.opts.signatureErr, ErrSignatureConfig)
}

// writePubKeyPEM writes a public key to a PEM file
func writePubKeyPEM(t *testing.T, path string, key crypto.PublicKey) {
	t.Helper()

	pubBytes, err := x509.MarshalPKIXPublicKey(key)
	require.NoError(t, err)

	pemBlock := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubBytes,
	}

	pemData := pem.EncodeToMemory(pemBlock)
	require.NoError(t, os.WriteFile(path, pemData, 0o644))
}

func BenchmarkMultiKeyVerification_2Keys(b *testing.B) {
	dir := b.TempDir()

	// Generate two keys
	pub1, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(b, err)

	pub2, priv2, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(b, err)

	// Create and sign plugin with second key
	pluginPath := filepath.Join(dir, "bench.so")
	pluginData := make([]byte, 1<<20) // 1MB
	hash := sha256.Sum256(pluginData)
	sig := ed25519.Sign(priv2, hash[:])

	require.NoError(b, os.WriteFile(pluginPath, pluginData, 0o644))
	require.NoError(b, os.WriteFile(pluginPath+".sig", sig, 0o644))

	b.ResetTimer()
	for b.Loop() {
		_ = verifySignatureMultiKey([]crypto.PublicKey{pub1, pub2}, pluginData, pluginPath+".sig")
	}
}

func BenchmarkMultiKeyVerification_5Keys(b *testing.B) {
	dir := b.TempDir()

	// Generate five keys, sign with the last one
	keys := make([]crypto.PublicKey, 5)
	var priv ed25519.PrivateKey

	for i := 0; i < 5; i++ {
		pub, p, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(b, err)
		keys[i] = pub
		if i == 4 {
			priv = p
		}
	}

	// Create and sign plugin with last key
	pluginPath := filepath.Join(dir, "bench.so")
	pluginData := make([]byte, 1<<20) // 1MB
	hash := sha256.Sum256(pluginData)
	sig := ed25519.Sign(priv, hash[:])

	require.NoError(b, os.WriteFile(pluginPath, pluginData, 0o644))
	require.NoError(b, os.WriteFile(pluginPath+".sig", sig, 0o644))

	b.ResetTimer()
	for b.Loop() {
		_ = verifySignatureMultiKey(keys, pluginData, pluginPath+".sig")
	}
}
