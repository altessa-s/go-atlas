// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package hash_test

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/encoding/hash"
)

func TestSHA256(t *testing.T) {
	input := "hello world"
	expectedBytes := sha256.Sum256([]byte(input))
	expectedHex := hex.EncodeToString(expectedBytes[:])

	t.Run("SHA256HexBytes", func(t *testing.T) {
		got := hash.SHA256HexBytes([]byte(input))
		require.Equal(t, expectedHex, got)
	})

	t.Run("SHA256HexString", func(t *testing.T) {
		got := hash.SHA256HexString(input)
		require.Equal(t, expectedHex, got)

		// Empty string check
		require.Equal(t, hash.SHA256HexBytes(nil), hash.SHA256HexString(""), "Empty string hash mismatch")
	})

	t.Run("SHA256HexWithPrefix", func(t *testing.T) {
		prefix := "sha256:"
		got := hash.SHA256HexWithPrefix(prefix, input)
		require.Equal(t, prefix+expectedHex, got)

		// Empty string with prefix
		emptyHash := hash.SHA256HexBytes(nil)
		require.Equal(t, prefix+emptyHash, hash.SHA256HexWithPrefix(prefix, ""))
	})

	t.Run("SHA256HexWithSalt", func(t *testing.T) {
		data := []byte(input)
		salt := []byte("random-salt")

		salted := hash.SHA256HexWithSalt(data, salt)
		unsalted := hash.SHA256HexBytes(data)

		require.NotEqual(t, unsalted, salted, "salted hash should differ from unsalted hash")

		// Same inputs produce the same hash (deterministic).
		require.Equal(t, salted, hash.SHA256HexWithSalt(data, salt), "SHA256HexWithSalt not deterministic")

		// Different salt produces a different hash.
		other := hash.SHA256HexWithSalt(data, []byte("other-salt"))
		require.NotEqual(t, salted, other, "different salts should produce different hashes")

		// Verify against reference: sha256(salt || data).
		ref := sha256.New()
		ref.Write(salt)
		ref.Write(data)
		want := hex.EncodeToString(ref.Sum(nil))
		require.Equal(t, want, salted)
	})

	t.Run("SHA256HexStringWithSalt", func(t *testing.T) {
		salt := "random-salt"

		salted := hash.SHA256HexStringWithSalt(input, salt)
		unsalted := hash.SHA256HexString(input)

		require.NotEqual(t, unsalted, salted, "salted hash should differ from unsalted hash")

		// Must match the []byte variant.
		byteSalted := hash.SHA256HexWithSalt([]byte(input), []byte(salt))
		require.Equal(t, byteSalted, salted, "string and byte variants disagree")

		// Empty data and salt.
		empty := hash.SHA256HexStringWithSalt("", "")
		require.Equal(t, hash.SHA256HexBytes(nil), empty, "empty salt+data should equal unsalted empty")
	})
}

func TestSHA256Hasher(t *testing.T) {
	t.Parallel()

	t.Run("single_write", func(t *testing.T) {
		t.Parallel()
		input := []byte("hello world")
		h := hash.NewSHA256Hasher()
		n, err := h.Write(input)
		require.NoError(t, err)
		require.Equal(t, len(input), n)
		require.Equal(t, hash.SHA256HexBytes(input), h.SumHex())
	})

	t.Run("multi_write_equals_single", func(t *testing.T) {
		t.Parallel()
		h := hash.NewSHA256Hasher()
		for _, p := range []string{"hello", " ", "world"} {
			_, err := h.Write([]byte(p))
			require.NoError(t, err)
		}
		require.Equal(t, hash.SHA256HexBytes([]byte("hello world")), h.SumHex())
	})

	t.Run("io_copy", func(t *testing.T) {
		t.Parallel()
		input := "hello world"
		h := hash.NewSHA256Hasher()
		_, err := io.Copy(h, strings.NewReader(input))
		require.NoError(t, err)
		require.Equal(t, hash.SHA256HexBytes([]byte(input)), h.SumHex())
	})

	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		h := hash.NewSHA256Hasher()
		require.Equal(t, hash.SHA256HexBytes(nil), h.SumHex())
	})

	t.Run("sum_hex_is_stable", func(t *testing.T) {
		t.Parallel()
		h := hash.NewSHA256Hasher()
		_, err := h.Write([]byte("data"))
		require.NoError(t, err)
		require.Equal(t, h.SumHex(), h.SumHex(), "SumHex must not reset state")
	})

}

func TestHexSum(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{"hello_world", "hello world"},
		{"empty", ""},
		{"binary_data", "\x00\x01\x02"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := sha256.New()
			h.Write([]byte(tc.input))
			got := hash.HexSum(h)
			want := hash.SHA256HexBytes([]byte(tc.input))
			require.Equal(t, want, got)
		})
	}

	t.Run("matches_sha256_hasher", func(t *testing.T) {
		t.Parallel()
		input := "test data"
		sh := hash.NewSHA256Hasher()
		sh.Write([]byte(input))
		stdH := sha256.New()
		stdH.Write([]byte(input))
		require.Equal(t, hash.HexSum(stdH), sh.SumHex())
	})
}
