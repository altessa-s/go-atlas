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
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// signingKey is one key pair plus the ability to sign with it, so a target can
// state both halves of the contract without re-deriving keys per execution.
type signingKey struct {
	public crypto.PublicKey
	sign   func(tb testing.TB, data []byte) []byte
}

// fuzzKeys mints one key of each supported algorithm. Generating them once at
// target setup keeps the budget on the verifier rather than on RSA keygen.
func fuzzKeys(tb testing.TB) []signingKey {
	tb.Helper()

	edPub, edPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(tb, err)

	ecPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(tb, err)

	return []signingKey{
		{
			public: edPub,
			sign: func(tb testing.TB, data []byte) []byte {
				tb.Helper()
				digest := sha256.Sum256(data)
				return ed25519.Sign(edPriv, digest[:])
			},
		},
		{
			public: &ecPriv.PublicKey,
			sign: func(tb testing.TB, data []byte) []byte {
				tb.Helper()
				digest := sha256.Sum256(data)
				sig, err := ecdsa.SignASN1(rand.Reader, ecPriv, digest[:])
				require.NoError(tb, err)
				return sig
			},
		},
	}
}

// writeSig writes sig to a fresh file and returns its path.
func writeSig(tb testing.TB, sig []byte) string {
	tb.Helper()

	path := filepath.Join(tb.TempDir(), "plugin.so.sig")
	require.NoError(tb, os.WriteFile(path, sig, 0o600))

	return path
}

// FuzzVerifySignatureRejectsForgedSignatures is the plugin supply-chain oracle.
//
// A plugin is loaded into the host process and runs with its privileges, so the
// signature check is the only thing between a tampered .so on disk and
// arbitrary code inside the service. The fuzzer does not hold the private key,
// so any signature bytes it produces are by definition unsigned — and Verify
// returning nil for them would mean the check can be talked past with a file
// anyone able to write next to the plugin can create.
func FuzzVerifySignatureRejectsForgedSignatures(f *testing.F) {
	f.Add([]byte("plugin bytes"), []byte{})
	f.Add([]byte(""), []byte("not a signature"))
	f.Add([]byte("plugin bytes"), make([]byte, 64)) // Right length for Ed25519, wrong content.
	f.Add([]byte("plugin bytes"), make([]byte, 0, 8))

	keys := fuzzKeys(f)

	f.Fuzz(func(t *testing.T, pluginData, signature []byte) {
		sigPath := writeSig(t, signature)

		for _, key := range keys {
			err := verifySignature(key.public, pluginData, sigPath)
			require.Error(t, err,
				"a signature the caller invented verified against %T", key.public)
			require.ErrorIs(t, err, ErrSignatureInvalid)
		}
	})
}

// FuzzVerifySignatureAcceptsItsOwnSignature is the same boundary from the other
// side: whatever the signer produced over these exact bytes must verify.
//
// Plugin payloads are arbitrary binaries, so the interesting inputs are the
// degenerate ones — an empty file, a single byte, something large enough to
// cross a hashing block boundary — where a digest computed over the wrong slice
// would start rejecting genuine plugins on a deploy nobody changed.
func FuzzVerifySignatureAcceptsItsOwnSignature(f *testing.F) {
	f.Add([]byte("plugin bytes"))
	f.Add([]byte(""))
	f.Add([]byte{0})
	f.Add(make([]byte, 4096))

	keys := fuzzKeys(f)

	f.Fuzz(func(t *testing.T, pluginData []byte) {
		for _, key := range keys {
			sigPath := writeSig(t, key.sign(t, pluginData))
			require.NoError(t, verifySignature(key.public, pluginData, sigPath),
				"a freshly signed plugin failed its own verification with %T", key.public)
		}
	})
}

// FuzzVerifySignatureRejectsMutatedPayload pins what a signature is for: the
// bytes it authenticates are the exact ones that were signed.
//
// This is the attack the check exists to stop — a plugin edited in place after
// it was signed — and it is distinct from the forged-signature case above: here
// the signature is genuine, and only the payload moved.
func FuzzVerifySignatureRejectsMutatedPayload(f *testing.F) {
	f.Add([]byte("plugin bytes"), uint16(0), byte(1))
	f.Add([]byte("plugin bytes"), uint16(5), byte(0xff))
	f.Add([]byte{0x7f}, uint16(0), byte(0x80))

	keys := fuzzKeys(f)

	f.Fuzz(func(t *testing.T, pluginData []byte, position uint16, delta byte) {
		if len(pluginData) == 0 {
			t.Skip("there is nothing to mutate in an empty payload")
		}
		if delta == 0 {
			t.Skip("a zero delta is not a mutation")
		}

		mutated := make([]byte, len(pluginData))
		copy(mutated, pluginData)
		mutated[int(position)%len(mutated)] += delta

		for _, key := range keys {
			sigPath := writeSig(t, key.sign(t, pluginData))

			err := verifySignature(key.public, mutated, sigPath)
			require.Error(t, err, "a mutated plugin verified against %T", key.public)
			require.True(t, errors.Is(err, ErrSignatureInvalid),
				"a mutated plugin failed for the wrong reason: %v", err)
		}
	})
}
