// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package main

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGenerateKey_Algorithms confirms generateKey returns the expected concrete
// type for each supported algorithm and rejects unknown algorithms / out-of-range
// RSA bit sizes.
func TestGenerateKey_Algorithms(t *testing.T) {
	t.Parallel()

	t.Run("ed25519", func(t *testing.T) {
		t.Parallel()
		priv, err := generateKey(algEd25519, 0)
		require.NoError(t, err)
		_, ok := priv.(ed25519.PrivateKey)
		require.True(t, ok, "expected ed25519.PrivateKey, got %T", priv)
	})

	t.Run("ecdsa", func(t *testing.T) {
		t.Parallel()
		priv, err := generateKey(algECDSA, 0)
		require.NoError(t, err)
		_, ok := priv.(*ecdsa.PrivateKey)
		require.True(t, ok, "expected *ecdsa.PrivateKey, got %T", priv)
	})

	t.Run("rsa_default_bits", func(t *testing.T) {
		t.Parallel()
		priv, err := generateKey(algRSA, rsaDefaultBits)
		require.NoError(t, err)
		k, ok := priv.(*rsa.PrivateKey)
		require.True(t, ok, "expected *rsa.PrivateKey, got %T", priv)
		require.Equal(t, rsaDefaultBits, k.N.BitLen())
	})

	t.Run("rsa_below_min_rejected", func(t *testing.T) {
		t.Parallel()
		_, err := generateKey(algRSA, 1024)
		require.Error(t, err)
	})

	t.Run("rsa_above_max_rejected", func(t *testing.T) {
		t.Parallel()
		_, err := generateKey(algRSA, 8192)
		require.Error(t, err)
	})

	t.Run("unknown_alg_rejected", func(t *testing.T) {
		t.Parallel()
		_, err := generateKey("dsa", 0)
		require.Error(t, err)
	})
}

// TestWriteKeyPair_RoundTrip generates each algorithm's key pair, writes it
// through writeKeyPair, then re-reads via the production loadPrivateKey /
// loadPublicKey and verifies sign+verify round-trip on a small payload. This
// is the strongest guarantee that keygen output is interchangeable with the
// existing sign/verify flow.
func TestWriteKeyPair_RoundTrip(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		alg  string
		bits int
	}{
		{"ed25519", algEd25519, 0},
		{"ecdsa", algECDSA, 0},
		{"rsa", algRSA, rsaMinBits},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			privPath := filepath.Join(dir, "private.pem")
			pubPath := filepath.Join(dir, "public.pem")

			priv, err := generateKey(tc.alg, tc.bits)
			require.NoError(t, err)
			require.NoError(t, writeKeyPair(priv, privPath, pubPath, false))

			// Permission contract: private 0o600, public 0o644.
			privStat, err := os.Stat(privPath)
			require.NoError(t, err)
			require.Equal(t, privFileMode, privStat.Mode().Perm(),
				"private key must be %#o, got %#o", privFileMode, privStat.Mode().Perm())
			pubStat, err := os.Stat(pubPath)
			require.NoError(t, err)
			require.Equal(t, pubFileMode, pubStat.Mode().Perm(),
				"public key must be %#o, got %#o", pubFileMode, pubStat.Mode().Perm())

			// Round-trip: load via production helpers, sign+verify a fake plugin.
			loadedPriv, err := loadPrivateKey(privPath)
			require.NoError(t, err)
			loadedPub, err := loadPublicKey(pubPath)
			require.NoError(t, err)

			pluginPath := filepath.Join(dir, "fake.so")
			require.NoError(t, os.WriteFile(pluginPath, []byte("fake plugin payload"), 0o644))
			require.NoError(t, signFile(pluginPath, loadedPriv))
			require.NoError(t, verifyFile(pluginPath, loadedPub))
		})
	}
}

// TestWriteKeyPair_RefusesOverwriteWithoutForce confirms the no-clobber default:
// once a key file exists, a second writeKeyPair without force surfaces
// errKeyFileExists rather than silently overwriting.
func TestWriteKeyPair_RefusesOverwriteWithoutForce(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	privPath := filepath.Join(dir, "private.pem")
	pubPath := filepath.Join(dir, "public.pem")

	priv, err := generateKey(algEd25519, 0)
	require.NoError(t, err)
	require.NoError(t, writeKeyPair(priv, privPath, pubPath, false))

	// Capture original bytes to prove they are not overwritten on the
	// rejected second attempt.
	origPriv, err := os.ReadFile(privPath)
	require.NoError(t, err)

	priv2, err := generateKey(algEd25519, 0)
	require.NoError(t, err)
	err = writeKeyPair(priv2, privPath, pubPath, false)
	require.ErrorIs(t, err, errKeyFileExists)

	currentPriv, err := os.ReadFile(privPath)
	require.NoError(t, err)
	require.Equal(t, origPriv, currentPriv, "private file must be untouched after refused overwrite")
}

// TestWriteKeyPair_ForceOverwrites confirms -force semantics: a second
// writeKeyPair with force=true succeeds and the on-disk bytes change.
func TestWriteKeyPair_ForceOverwrites(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	privPath := filepath.Join(dir, "private.pem")
	pubPath := filepath.Join(dir, "public.pem")

	priv1, err := generateKey(algEd25519, 0)
	require.NoError(t, err)
	require.NoError(t, writeKeyPair(priv1, privPath, pubPath, false))
	origPriv, err := os.ReadFile(privPath)
	require.NoError(t, err)

	priv2, err := generateKey(algEd25519, 0)
	require.NoError(t, err)
	require.NoError(t, writeKeyPair(priv2, privPath, pubPath, true))

	newPriv, err := os.ReadFile(privPath)
	require.NoError(t, err)
	require.NotEqual(t, origPriv, newPriv, "force overwrite must replace the private file contents")

	// Permissions must still be 0o600 after the truncating overwrite.
	st, err := os.Stat(privPath)
	require.NoError(t, err)
	require.Equal(t, privFileMode, st.Mode().Perm())
}

// TestWriteKeyPair_RollbackOnPublicFailure verifies the half-state guarantee:
// if the public file cannot be written (here: a pre-existing pub path without
// force), the private file is rolled back so the caller doesn't end up with
// an orphan private key whose public half is unknown.
func TestWriteKeyPair_RollbackOnPublicFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	privPath := filepath.Join(dir, "private.pem")
	pubPath := filepath.Join(dir, "public.pem")

	// Pre-seed only the public path so the private write succeeds and the
	// public write trips errKeyFileExists.
	require.NoError(t, os.WriteFile(pubPath, []byte("pre-existing"), 0o644))

	priv, err := generateKey(algEd25519, 0)
	require.NoError(t, err)
	err = writeKeyPair(priv, privPath, pubPath, false)
	require.Error(t, err)
	require.ErrorIs(t, err, errKeyFileExists)

	_, statErr := os.Stat(privPath)
	require.ErrorIs(t, statErr, os.ErrNotExist,
		"private file must be rolled back when public write fails")
}

// TestRunKeygen_RejectsSamePaths guards against a foot-gun: pointing -priv-out
// and -pub-out at the same file would silently overwrite the private with the
// public half on the second write. The CLI must reject the configuration up
// front.
func TestRunKeygen_RejectsSamePaths(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	same := filepath.Join(dir, "key.pem")
	err := runKeygen([]string{
		"-alg", algEd25519,
		"-priv-out", same,
		"-pub-out", same,
	})
	require.Error(t, err)
}

// TestRunKeygen_UnknownAlgFails sanity-checks that bad -alg propagates out of
// runKeygen with a non-nil error.
func TestRunKeygen_UnknownAlgFails(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	err := runKeygen([]string{
		"-alg", "dsa",
		"-priv-out", filepath.Join(dir, "p.pem"),
		"-pub-out", filepath.Join(dir, "P.pem"),
	})
	require.Error(t, err)
}
