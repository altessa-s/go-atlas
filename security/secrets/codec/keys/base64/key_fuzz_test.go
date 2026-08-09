// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package base64_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/security/secrets/codec/keys/base64"
)

// FuzzEncodeDecodeRoundTrip pins that a key survives the encoding it is stored
// under.
//
// Secret keys are provider-scoped names that go through this codec on the way
// into a backend and back out on lookup. A key that decodes to something other
// than what was encoded means a secret written under one name is looked up
// under another — a miss that reads as "secret not configured", far from the
// codec that caused it.
func FuzzEncodeDecodeRoundTrip(f *testing.F) {
	f.Add("database/password")
	f.Add("")
	f.Add("with spaces and / slashes")
	f.Add("ключ")
	f.Add("\x00\xff")

	decoder := base64.NewKeyDecoder()

	f.Fuzz(func(t *testing.T, key string) {
		encoded, err := decoder.Encode(key)
		require.NoError(t, err, "Encode refused a key it is expected to carry")

		decoded, err := decoder.Decode(encoded)
		require.NoError(t, err, "Decode refused this codec's own output: %q", encoded)
		require.Equal(t, key, decoded, "the key changed on its way through the codec")
	})
}

// FuzzDecodeArbitraryInput covers the direction that meets stored data: text
// written by an older version, by a different tool, or corrupted in transit.
//
// The only claim is that Decode either refuses the input or produces a value
// that re-encodes to the same text. A decoder more permissive than its encoder
// gives one key two spellings, which is how a rotation ends up writing to one
// and reading from the other.
func FuzzDecodeArbitraryInput(f *testing.F) {
	f.Add("ZGF0YWJhc2UvcGFzc3dvcmQ=")
	f.Add("")
	f.Add("not base64 at all!!")
	f.Add("ZGF0YWJhc2UvcGFzc3dvcmQ") // Unpadded.
	f.Add("=")
	f.Add("////")

	decoder := base64.NewKeyDecoder()

	f.Fuzz(func(t *testing.T, encoded string) {
		decoded, err := decoder.Decode(encoded)
		if err != nil {
			require.Empty(t, decoded, "a rejected key must not also be returned")
			return
		}

		reEncoded, err := decoder.Encode(decoded)
		require.NoError(t, err)

		again, err := decoder.Decode(reEncoded)
		require.NoError(t, err, "the codec could not read back its own encoding of %q", decoded)
		require.Equal(t, decoded, again, "decoding is not stable for %q", encoded)
	})
}
