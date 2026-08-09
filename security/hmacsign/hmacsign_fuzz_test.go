// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package hmacsign_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/security/hmacsign"
)

// fuzzSecret is the shared secret the fuzzer does not have. Every negative
// assertion below rests on that: an input the fuzzer invented cannot carry a
// digest keyed by a secret it never saw.
var fuzzSecret = []byte("fuzz-shared-secret")

// schemeFor picks one of the two built-in wire formats. They parse headers
// completely differently — GitHub reads a single "sha256=<hex>" token, Stripe a
// comma-separated key/value list with a timestamp — so both belong in the same
// corpus rather than in two near-identical targets.
func schemeFor(sel uint8) hmacsign.Scheme {
	if sel%2 == 0 {
		return hmacsign.GitHub()
	}
	return hmacsign.Stripe()
}

// FuzzVerifyRejectsUnsignedHeader is the auth-bypass oracle: no header value a
// caller can invent may authenticate a body.
//
// The signature header is entirely attacker-controlled — it arrives on an
// inbound webhook — and Verify returning nil is the single decision that admits
// the request. So the property is not "the parser does not panic" but "the
// parser cannot be talked into a nil error", which is exactly what a header
// grammar with optional fields, duplicate keys, and lenient splitting risks:
// Stripe's parser skips parts it cannot Cut and accumulates every v1 it sees.
func FuzzVerifyRejectsUnsignedHeader(f *testing.F) {
	f.Add(uint8(0), "sha256=deadbeef", []byte("{}"))
	f.Add(uint8(0), "sha256=", []byte(""))
	f.Add(uint8(0), "", []byte("body"))
	f.Add(uint8(1), "t=1700000000,v1=deadbeef", []byte("{}"))
	f.Add(uint8(1), "t=1700000000,v1=,v1=,v1=", []byte("{}"))
	f.Add(uint8(1), "t=0,v0=deadbeef,v1=deadbeef", []byte(""))
	f.Add(uint8(1), ",,,=,==,", []byte("body"))

	f.Fuzz(func(t *testing.T, sel uint8, header string, body []byte) {
		// Tolerance off: a rejection must come from the signature failing to
		// match, not from a timestamp the fuzzer happened to make stale. That
		// removes the easy way to pass and leaves only the hard one.
		v := hmacsign.NewVerifier(schemeFor(sel), fuzzSecret, hmacsign.WithTolerance(0))

		require.Error(t, v.Verify(header, body),
			"Verify accepted a header that was never signed with the secret: scheme=%T header=%q", schemeFor(sel), header)
	})
}

// FuzzSignedHeaderVerifies is the same boundary from the other side: whatever
// the signer emits, the verifier must accept.
//
// The body is arbitrary bytes on purpose. Stripe's canonical message is
// "<unix>.<body>", so a body containing dots, commas or equals signs is exactly
// where a format that concatenates without framing would start disagreeing with
// the parser that takes it apart — and a signer and verifier that disagree
// reject legitimate traffic silently.
func FuzzSignedHeaderVerifies(f *testing.F) {
	f.Add(uint8(0), uint32(1700000000), []byte(`{"action":"opened"}`))
	f.Add(uint8(1), uint32(1700000000), []byte(`{"action":"opened"}`))
	f.Add(uint8(1), uint32(0), []byte("t=1,v1=deadbeef"))
	f.Add(uint8(1), uint32(4294967295), []byte(""))
	f.Add(uint8(0), uint32(1), []byte{0x00, 0xff, 0x2c, 0x3d, 0x2e})

	f.Fuzz(func(t *testing.T, sel uint8, unixSec uint32, body []byte) {
		scheme := schemeFor(sel)

		// A uint32 keeps the stamp inside 1970..2106, where a Unix second
		// round-trips exactly. Extreme int64 instants do not survive
		// time.Unix -> Unix() unchanged, and pinning that would assert
		// something about the clock rather than about the wire format.
		clock := func() time.Time { return time.Unix(int64(unixSec), 0) }

		signer := hmacsign.NewSigner(scheme, fuzzSecret, hmacsign.WithClock(clock))
		verifier := hmacsign.NewVerifier(scheme, fuzzSecret,
			hmacsign.WithClock(clock), hmacsign.WithTolerance(time.Minute))

		header := signer.Sign(body)
		require.NoError(t, verifier.Verify(header, body),
			"a freshly signed body failed its own verification: scheme=%T header=%q", scheme, header)
	})
}

// FuzzVerifyRejectsMutatedBody pins the point of a signature: the body it
// authenticates is the exact one that was signed.
//
// The header stays valid and only the body changes, so a verifier that reads
// the header, parses it successfully, and then compares against a message built
// from something other than the caller-supplied bytes would pass this by
// accident under the previous target and fail here.
func FuzzVerifyRejectsMutatedBody(f *testing.F) {
	f.Add(uint8(0), []byte("original"), []byte("tampered"))
	f.Add(uint8(1), []byte("original"), []byte("tampered"))
	f.Add(uint8(1), []byte(""), []byte("."))
	f.Add(uint8(0), []byte("a"), []byte("a "))

	f.Fuzz(func(t *testing.T, sel uint8, signed, received []byte) {
		if string(signed) == string(received) {
			t.Skip("identical bodies are the positive case, covered by FuzzSignedHeaderVerifies")
		}

		scheme := schemeFor(sel)
		clock := func() time.Time { return time.Unix(1700000000, 0) }

		signer := hmacsign.NewSigner(scheme, fuzzSecret, hmacsign.WithClock(clock))
		verifier := hmacsign.NewVerifier(scheme, fuzzSecret,
			hmacsign.WithClock(clock), hmacsign.WithTolerance(time.Minute))

		require.Error(t, verifier.Verify(signer.Sign(signed), received),
			"a signature made over %q authenticated %q instead", signed, received)
	})
}
