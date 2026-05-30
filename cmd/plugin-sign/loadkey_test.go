// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package main

import (
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestLoadPrivateKey_GarbagePEMSurfacesAllAttempts verifies the diagnostic
// contract of loadPrivateKey: when a PEM block contains data none of the
// supported parsers can decode, the returned error must mention every format
// tried (PKCS#8, PKCS#1, SEC1 EC, raw Ed25519 seed). A previous version
// returned a generic "failed to parse private key" with no breakdown,
// leaving users without a hint about what went wrong.
func TestLoadPrivateKey_GarbagePEMSurfacesAllAttempts(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "garbage.pem")

	// Length 7 is deliberate: not 32, so the raw-seed branch reports a
	// length mismatch we can match on, and not zero (would still parse-fail
	// on the asn.1 paths but produce different upstream messages).
	garbage := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: []byte("not-asn1"),
	})
	require.NoError(t, os.WriteFile(path, garbage, 0o600))

	_, err := loadPrivateKey(path)
	require.Error(t, err)

	got := err.Error()
	for _, want := range []string{"PKCS#8:", "PKCS#1:", "SEC1 EC:", "raw Ed25519 seed:"} {
		require.Contains(t, got, want,
			"error must surface the %s attempt; got: %s", want, got)
	}
	require.Contains(t, got, `PEM type "PRIVATE KEY"`,
		"error must surface the PEM block type for context; got: %s", got)
}

// TestLoadPrivateKey_NotPEM verifies the early-exit path: a file with no PEM
// block produces a focused error rather than running through every parser.
func TestLoadPrivateKey_NotPEM(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "plain.txt")
	require.NoError(t, os.WriteFile(path, []byte("hello, not a pem"), 0o600))

	_, err := loadPrivateKey(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no PEM block found")
	// And — by construction — none of the parser-attempt diagnostics fire.
	require.NotContains(t, err.Error(), "PKCS#8:",
		"early-exit error must not include parser attempts")
}
