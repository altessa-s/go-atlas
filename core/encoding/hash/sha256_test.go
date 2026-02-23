// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package hash_test

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/altessa-s/go-atlas/core/encoding/hash"
)

func TestSHA256(t *testing.T) {
	input := "hello world"
	expectedBytes := sha256.Sum256([]byte(input))
	expectedHex := hex.EncodeToString(expectedBytes[:])

	t.Run("SHA256HexBytes", func(t *testing.T) {
		got := hash.SHA256HexBytes([]byte(input))
		if got != expectedHex {
			t.Errorf("SHA256HexBytes() = %v, want %v", got, expectedHex)
		}
	})

	t.Run("SHA256HexString", func(t *testing.T) {
		got := hash.SHA256HexString(input)
		if got != expectedHex {
			t.Errorf("SHA256HexString() = %v, want %v", got, expectedHex)
		}

		// Empty string check
		if hash.SHA256HexString("") != hash.SHA256HexBytes(nil) {
			t.Error("Empty string hash mismatch")
		}
	})

	t.Run("SHA256HexWithPrefix", func(t *testing.T) {
		prefix := "sha256:"
		got := hash.SHA256HexWithPrefix(prefix, input)
		if got != prefix+expectedHex {
			t.Errorf("SHA256HexWithPrefix() = %v, want %v", got, prefix+expectedHex)
		}

		// Empty string with prefix
		emptyHash := hash.SHA256HexBytes(nil)
		if got := hash.SHA256HexWithPrefix(prefix, ""); got != prefix+emptyHash {
			t.Errorf("SHA256HexWithPrefix(prefix, empty) = %v, want %v", got, prefix+emptyHash)
		}
	})

	t.Run("SHA256HexWithSalt", func(t *testing.T) {
		data := []byte(input)
		salt := []byte("random-salt")

		salted := hash.SHA256HexWithSalt(data, salt)
		unsalted := hash.SHA256HexBytes(data)

		if salted == unsalted {
			t.Error("salted hash should differ from unsalted hash")
		}

		// Same inputs produce the same hash (deterministic).
		if got := hash.SHA256HexWithSalt(data, salt); got != salted {
			t.Errorf("SHA256HexWithSalt not deterministic: %v != %v", got, salted)
		}

		// Different salt produces a different hash.
		other := hash.SHA256HexWithSalt(data, []byte("other-salt"))
		if other == salted {
			t.Error("different salts should produce different hashes")
		}

		// Verify against reference: sha256(salt || data).
		ref := sha256.New()
		ref.Write(salt)
		ref.Write(data)
		want := hex.EncodeToString(ref.Sum(nil))
		if salted != want {
			t.Errorf("SHA256HexWithSalt() = %v, want %v", salted, want)
		}
	})

	t.Run("SHA256HexStringWithSalt", func(t *testing.T) {
		salt := "random-salt"

		salted := hash.SHA256HexStringWithSalt(input, salt)
		unsalted := hash.SHA256HexString(input)

		if salted == unsalted {
			t.Error("salted hash should differ from unsalted hash")
		}

		// Must match the []byte variant.
		byteSalted := hash.SHA256HexWithSalt([]byte(input), []byte(salt))
		if salted != byteSalted {
			t.Errorf("string and byte variants disagree: %v != %v", salted, byteSalted)
		}

		// Empty data and salt.
		empty := hash.SHA256HexStringWithSalt("", "")
		if empty != hash.SHA256HexBytes(nil) {
			t.Errorf("empty salt+data should equal unsalted empty: %v", empty)
		}
	})
}
