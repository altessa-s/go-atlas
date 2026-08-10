// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package codec

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

// FuzzNegotiateOnlyReturnsRegisteredCodecs is the content-negotiation oracle.
//
// The Accept header is caller-controlled and decides how a response is
// serialized. Negotiate returning a MIME type nobody registered — or a nil
// encoder alongside a nil error — leaves the caller writing a response with
// whatever it got, which is at best an empty body and at worst a nil
// dereference on a path an attacker chooses by sending a header.
func FuzzNegotiateOnlyReturnsRegisteredCodecs(f *testing.F) {
	f.Add("application/json")
	f.Add("*/*")
	f.Add("application/*")
	f.Add("")
	f.Add("application/json;q=0.1, application/xml;q=0.9")
	f.Add("application/json;q=notanumber")
	f.Add(",,,")
	f.Add("application/json, */*;q=0")
	f.Add("text/plain")

	registry := NewRegistry()

	f.Fuzz(func(t *testing.T, accept string) {
		encoder, mimeType, err := registry.Negotiate(accept)
		if err != nil {
			require.Nil(t, encoder, "a failed negotiation must not also return an encoder")
			require.Empty(t, mimeType)
			return
		}

		require.NotNil(t, encoder,
			"negotiation succeeded with no encoder, so the caller writes nothing: accept=%q", accept)

		registered := slices.Collect(registry.ListEncoders())
		require.Contains(t, registered, mimeType,
			"negotiation chose a MIME type nobody registered: %q (accept=%q)", mimeType, accept)
	})
}

// FuzzNegotiateIsDeterministic pins that one Accept header always resolves to
// one codec.
//
// The registry is read under a lock and consulted per request, so a result that
// varied between two identical headers would mean two clients sending the same
// Accept get different content types — and a cache keyed on the request would
// then serve one of them the other's encoding.
func FuzzNegotiateIsDeterministic(f *testing.F) {
	f.Add("application/json")
	f.Add("application/json;q=0.5, application/xml;q=0.5")
	f.Add("*/*")
	f.Add("")

	registry := NewRegistry()

	f.Fuzz(func(t *testing.T, accept string) {
		_, firstType, firstErr := registry.Negotiate(accept)
		_, secondType, secondErr := registry.Negotiate(accept)

		require.Equal(t, firstErr == nil, secondErr == nil,
			"negotiation succeeded once and failed once for %q", accept)
		require.Equal(t, firstType, secondType,
			"negotiation chose two different codecs for %q", accept)
	})
}
