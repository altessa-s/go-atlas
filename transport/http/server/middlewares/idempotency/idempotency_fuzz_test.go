// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// FuzzDefaultKeyValidatorAcceptsOnlyLowercaseUUIDv4 pins the exact shape an
// idempotency key must have.
//
// The key becomes part of a storage key, so what the validator accepts decides
// what a client can write into the keyspace. Accepting mixed case would give
// one logical key two entries — a retry that lands on the other one replays an
// operation that was supposed to be deduplicated — and accepting anything
// looser than a UUID would let a client choose a key that collides with
// another's.
//
// The rule is restated here rather than reused, so an oracle and its subject
// cannot agree on a mistake.
func FuzzDefaultKeyValidatorAcceptsOnlyLowercaseUUIDv4(f *testing.F) {
	f.Add("9f8a1f0e-3c4d-4b6a-8f2e-1a2b3c4d5e6f")
	f.Add("9F8A1F0E-3C4D-4B6A-8F2E-1A2B3C4D5E6F")
	f.Add("")
	f.Add("not-a-uuid")
	f.Add("9f8a1f0e3c4d4b6a8f2e1a2b3c4d5e6f")
	f.Add("9f8a1f0e-3c4d-1b6a-8f2e-1a2b3c4d5e6f") // Version 1, not 4.
	f.Add("9f8a1f0e-3c4d-4b6a-0f2e-1a2b3c4d5e6f") // Bad variant nibble.
	f.Add(" 9f8a1f0e-3c4d-4b6a-8f2e-1a2b3c4d5e6f")

	f.Fuzz(func(t *testing.T, key string) {
		err := DefaultKeyValidator(key)

		require.Equal(t, isLowercaseUUIDv4(key), err == nil,
			"the validator disagrees with the documented key shape: %q (err=%v)", key, err)
	})
}

// isLowercaseUUIDv4 is an independent statement of the accepted format:
// 8-4-4-4-12 lowercase hex, version nibble 4, variant nibble one of 8/9/a/b.
func isLowercaseUUIDv4(key string) bool {
	if len(key) != 36 {
		return false
	}
	for i, c := range key {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !strings.ContainsRune("0123456789abcdef", c) {
				return false
			}
		}
	}
	if key[14] != '4' {
		return false
	}
	return strings.ContainsRune("89ab", rune(key[19]))
}
