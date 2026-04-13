// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugins

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeSignatureFile writes raw sig bytes to path+".sig".
func writeSignatureFile(t *testing.T, soPath string, sig []byte) {
	t.Helper()
	require.NoError(t, os.WriteFile(soPath+sigExtension, sig, 0o644))
}

// marshalPublicKeyPEM encodes a public key to PKIX PEM.
func marshalPublicKeyPEM(t *testing.T, pub crypto.PublicKey) []byte {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(pub)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
}

// --- Ed25519 ---

func TestVerifySignature_Ed25519_OK(t *testing.T) {
	t.Parallel()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	data := []byte("plugin binary content")
	sig := ed25519.Sign(priv, data)

	sigPath := filepath.Join(t.TempDir(), "test.so.sig")
	require.NoError(t, os.WriteFile(sigPath, sig, 0o644))

	assert.NoError(t, verifySignature(pub, data, sigPath))
}

func TestVerifySignature_Ed25519_Bad(t *testing.T) {
	t.Parallel()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	data := []byte("plugin binary content")
	sig := ed25519.Sign(priv, data)
	sig[0] ^= 0xff // corrupt

	sigPath := filepath.Join(t.TempDir(), "test.so.sig")
	require.NoError(t, os.WriteFile(sigPath, sig, 0o644))

	assert.ErrorIs(t, verifySignature(pub, data, sigPath), ErrSignatureInvalid)
}

// --- ECDSA P-256 ---

func TestVerifySignature_ECDSA_OK(t *testing.T) {
	t.Parallel()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	data := []byte("plugin binary content")
	digest := sha256.Sum256(data)
	sig, err := ecdsa.SignASN1(rand.Reader, priv, digest[:])
	require.NoError(t, err)

	sigPath := filepath.Join(t.TempDir(), "test.so.sig")
	require.NoError(t, os.WriteFile(sigPath, sig, 0o644))

	assert.NoError(t, verifySignature(&priv.PublicKey, data, sigPath))
}

func TestVerifySignature_ECDSA_Bad(t *testing.T) {
	t.Parallel()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	data := []byte("plugin binary content")
	digest := sha256.Sum256(data)
	sig, err := ecdsa.SignASN1(rand.Reader, priv, digest[:])
	require.NoError(t, err)
	sig[len(sig)-1] ^= 0xff // corrupt

	sigPath := filepath.Join(t.TempDir(), "test.so.sig")
	require.NoError(t, os.WriteFile(sigPath, sig, 0o644))

	assert.ErrorIs(t, verifySignature(&priv.PublicKey, data, sigPath), ErrSignatureInvalid)
}

// --- RSA-PSS ---

func TestVerifySignature_RSA_OK(t *testing.T) {
	t.Parallel()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	data := []byte("plugin binary content")
	digest := sha256.Sum256(data)
	sig, err := rsa.SignPSS(rand.Reader, priv, crypto.SHA256, digest[:], nil)
	require.NoError(t, err)

	sigPath := filepath.Join(t.TempDir(), "test.so.sig")
	require.NoError(t, os.WriteFile(sigPath, sig, 0o644))

	assert.NoError(t, verifySignature(&priv.PublicKey, data, sigPath))
}

func TestVerifySignature_RSA_Bad(t *testing.T) {
	t.Parallel()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	data := []byte("plugin binary content")
	digest := sha256.Sum256(data)
	sig, err := rsa.SignPSS(rand.Reader, priv, crypto.SHA256, digest[:], nil)
	require.NoError(t, err)
	sig[0] ^= 0xff // corrupt

	sigPath := filepath.Join(t.TempDir(), "test.so.sig")
	require.NoError(t, os.WriteFile(sigPath, sig, 0o644))

	assert.ErrorIs(t, verifySignature(&priv.PublicKey, data, sigPath), ErrSignatureInvalid)
}

// --- Missing .sig ---

func TestVerifySignature_MissingSigFile(t *testing.T) {
	t.Parallel()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	err = verifySignature(pub, []byte("data"), "/nonexistent/test.so.sig")
	assert.ErrorIs(t, err, os.ErrNotExist)
}

// --- Unsupported key type ---

type fakePublicKey struct{}

func TestVerifySignature_UnsupportedKey(t *testing.T) {
	t.Parallel()
	sigPath := filepath.Join(t.TempDir(), "test.so.sig")
	require.NoError(t, os.WriteFile(sigPath, []byte("sig"), 0o644))

	err := verifySignature(fakePublicKey{}, []byte("data"), sigPath)
	assert.ErrorIs(t, err, ErrSignatureConfig)
}

// --- PEM parsing ---

func TestParsePublicKeyPEM_Ed25519(t *testing.T) {
	t.Parallel()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	pemData := marshalPublicKeyPEM(t, pub)
	parsed, err := parsePublicKeyPEM(pemData)
	require.NoError(t, err)
	assert.IsType(t, ed25519.PublicKey{}, parsed)
}

func TestParsePublicKeyPEM_ECDSA(t *testing.T) {
	t.Parallel()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	pemData := marshalPublicKeyPEM(t, &priv.PublicKey)
	parsed, err := parsePublicKeyPEM(pemData)
	require.NoError(t, err)
	assert.IsType(t, &ecdsa.PublicKey{}, parsed)
}

func TestParsePublicKeyPEM_RSA(t *testing.T) {
	t.Parallel()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	pemData := marshalPublicKeyPEM(t, &priv.PublicKey)
	parsed, err := parsePublicKeyPEM(pemData)
	require.NoError(t, err)
	assert.IsType(t, &rsa.PublicKey{}, parsed)
}

func TestParsePublicKeyPEM_InvalidPEM(t *testing.T) {
	t.Parallel()
	_, err := parsePublicKeyPEM([]byte("not a PEM"))
	assert.ErrorIs(t, err, ErrSignatureConfig)
}

func TestParsePublicKeyPEM_WrongBlockType(t *testing.T) {
	t.Parallel()
	pemData := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("x")})
	_, err := parsePublicKeyPEM(pemData)
	assert.ErrorIs(t, err, ErrSignatureConfig)
}

// --- Manager integration ---

func TestManager_VerifyPluginSignature_Disabled(t *testing.T) {
	t.Parallel()
	mgr := NewManager()
	// signature.pubKey is nil by default → no-op.
	err := mgr.verifyPluginSignature("test.so", "/path/test.so", []byte("data"), "abc")
	assert.NoError(t, err)
}

func TestManager_VerifyPluginSignature_RequireMissing(t *testing.T) {
	t.Parallel()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	mgr := NewManager(WithSignature(SignatureOptions{
		Mode:      SignatureRequire,
		PublicKey: pub,
	}))
	t.Cleanup(func() { _ = mgr.Close() })

	err = mgr.verifyPluginSignature("test.so", "/nonexistent/test.so", []byte("data"), "abc")
	assert.ErrorIs(t, err, ErrSignatureMissing)

	// Should be quarantined.
	mgr.quarantineMu.RLock()
	_, ok := mgr.quarantine["test.so"]
	mgr.quarantineMu.RUnlock()
	assert.True(t, ok)
}

func TestManager_VerifyPluginSignature_WarnMissing(t *testing.T) {
	t.Parallel()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	mgr := NewManager(
		WithLogger(logger),
		WithSignature(SignatureOptions{
			Mode:      SignatureWarn,
			PublicKey: pub,
		}),
	)
	t.Cleanup(func() { _ = mgr.Close() })

	err = mgr.verifyPluginSignature("test.so", "/nonexistent/test.so", []byte("data"), "abc")
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "missing")

	// Should NOT be quarantined.
	mgr.quarantineMu.RLock()
	_, ok := mgr.quarantine["test.so"]
	mgr.quarantineMu.RUnlock()
	assert.False(t, ok)
}

func TestManager_VerifyPluginSignature_ValidSignature(t *testing.T) {
	t.Parallel()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	data := []byte("plugin binary")
	sig := ed25519.Sign(priv, data)

	dir := t.TempDir()
	soPath := filepath.Join(dir, "good.so")
	writeSignatureFile(t, soPath, sig)

	mgr := NewManager(WithSignature(SignatureOptions{
		Mode:      SignatureRequire,
		PublicKey: pub,
	}))
	t.Cleanup(func() { _ = mgr.Close() })

	err = mgr.verifyPluginSignature("good.so", soPath, data, "fakehash")
	assert.NoError(t, err)
}

func TestManager_VerifyPluginSignature_InvalidSignature(t *testing.T) {
	t.Parallel()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	data := []byte("plugin binary")
	sig := ed25519.Sign(priv, data)
	sig[0] ^= 0xff // corrupt

	dir := t.TempDir()
	soPath := filepath.Join(dir, "bad.so")
	writeSignatureFile(t, soPath, sig)

	mgr := NewManager(WithSignature(SignatureOptions{
		Mode:      SignatureRequire,
		PublicKey: pub,
	}))
	t.Cleanup(func() { _ = mgr.Close() })

	err = mgr.verifyPluginSignature("bad.so", soPath, data, "fakehash")
	assert.ErrorIs(t, err, ErrSignatureInvalid)

	// Quarantined.
	mgr.quarantineMu.RLock()
	_, ok := mgr.quarantine["bad.so"]
	mgr.quarantineMu.RUnlock()
	assert.True(t, ok)
}

// --- WithSignature deferred error ---

func TestWithSignature_DeferredError(t *testing.T) {
	t.Parallel()
	mgr := NewManager(WithSignature(SignatureOptions{
		Mode:          SignatureRequire,
		PublicKeyPath: "/nonexistent/key.pem",
	}))
	assert.NotNil(t, mgr.opts.signatureErr)
}

// --- loadPublicKey ---

func TestLoadPublicKey_Disabled(t *testing.T) {
	t.Parallel()
	key, err := loadPublicKey(SignatureOptions{Mode: SignatureDisabled})
	assert.NoError(t, err)
	assert.Nil(t, key)
}

func TestLoadPublicKey_DirectKey(t *testing.T) {
	t.Parallel()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	key, err := loadPublicKey(SignatureOptions{Mode: SignatureRequire, PublicKey: pub})
	assert.NoError(t, err)
	assert.Equal(t, pub, key)
}

func TestLoadPublicKey_FromPEMFile(t *testing.T) {
	t.Parallel()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	pemPath := filepath.Join(t.TempDir(), "key.pem")
	require.NoError(t, os.WriteFile(pemPath, marshalPublicKeyPEM(t, pub), 0o644))

	key, err := loadPublicKey(SignatureOptions{
		Mode:          SignatureRequire,
		PublicKeyPath: pemPath,
	})
	assert.NoError(t, err)
	assert.Equal(t, pub, key)
}

func TestLoadPublicKey_NoKeyConfigured(t *testing.T) {
	t.Parallel()
	_, err := loadPublicKey(SignatureOptions{Mode: SignatureRequire})
	assert.ErrorIs(t, err, ErrSignatureConfig)
}
