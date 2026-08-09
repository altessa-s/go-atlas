// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package json_test

import (
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"

	jsoncodec "github.com/altessa-s/go-atlas/security/secrets/codec/values/json"
)

// credentials stands in for the typed secret a caller actually stores — the
// shape that makes this codec generic rather than a string round trip.
// omitempty is deliberately absent from Extra: it would drop an
// empty-but-present map on re-encode, so a payload naming the field decodes to
// a non-nil empty map once and to nil the next time round. That is
// encoding/json's rule about this fixture's own tags, not something the codec
// could get right or wrong.
type credentials struct {
	User      string            `json:"user"`
	Password  string            `json:"password"`
	Extra     map[string]string `json:"extra"`
	Rotations int               `json:"rotations"`
}

// FuzzDecodeArbitraryPayload covers the direction that meets stored data.
//
// A secret value comes back from a backend that something else may have
// written: an older release with a different struct, an operator with a text
// editor, a provider that returned an error page. Decode has to either refuse
// it or produce a value, and never do both — a partially populated struct
// returned alongside an error is the shape a caller uses by accident.
func FuzzDecodeArbitraryPayload(f *testing.F) {
	f.Add(`{"user":"svc","password":"hunter2"}`)
	f.Add(`{}`)
	f.Add(``)
	f.Add(`null`)
	f.Add(`[1,2,3]`)
	f.Add(`{"user":123}`)
	f.Add(`{"rotations":"not a number"}`)
	f.Add(`{"user":"a"}{"user":"b"}`)
	f.Add(`{"extra":{"k":"v"}}`)

	decoder := jsoncodec.NewValueDecoder[credentials]()

	f.Fuzz(func(t *testing.T, payload string) {
		value, err := decoder.Decode([]byte(payload))
		if err != nil {
			require.Zero(t, value, "a rejected payload must not also yield a value")
			return
		}

		// Anything accepted has to survive being written back out, or the same
		// secret has two encodings and a rotation can read a value it did not
		// write.
		encoded, err := decoder.Encode(value)
		require.NoError(t, err)

		again, err := decoder.Decode(encoded)
		require.NoError(t, err, "the codec could not read back its own encoding: %s", encoded)
		require.Equal(t, value, again, "decoding is not stable for %q", payload)
	})
}

// FuzzEncodeDecodeRoundTrip pins the direction a caller controls: a typed
// secret must come back as itself.
//
// The fields carry credentials, so a value that changes across the round trip
// is a service authenticating with something other than what was configured —
// and the failure appears at the remote end as a permission error, nowhere near
// the codec.
func FuzzEncodeDecodeRoundTrip(f *testing.F) {
	f.Add("svc", "hunter2", 0)
	f.Add("", "", -1)
	f.Add(`{"nested":"json"}`, "p\"a\\s's", 42)
	f.Add("пользователь", "пароль", 7)

	decoder := jsoncodec.NewValueDecoder[credentials]()

	f.Fuzz(func(t *testing.T, user, password string, rotations int) {
		if !utf8.ValidString(user) || !utf8.ValidString(password) {
			// JSON substitutes U+FFFD for anything that is not UTF-8, so a
			// secret carrying raw bytes cannot survive a JSON codec. Callers
			// storing binary secrets need a binary codec, not this one.
			t.Skip("JSON strings are UTF-8; raw bytes cannot round-trip")
		}

		original := credentials{User: user, Password: password, Rotations: rotations}

		encoded, err := decoder.Encode(original)
		require.NoError(t, err)

		decoded, err := decoder.Decode(encoded)
		require.NoError(t, err, "the codec refused its own output: %s", encoded)
		require.Equal(t, original, decoded, "the secret changed on its way through the codec")
	})
}
