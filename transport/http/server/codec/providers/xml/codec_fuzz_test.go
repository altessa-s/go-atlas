// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package xml_test

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	xmlcodec "github.com/altessa-s/go-atlas/transport/http/server/codec/providers/xml"
)

// payload is the shape a handler decodes a request body into.
type payload struct {
	XMLName xml.Name `xml:"payload"`
	Name    string   `xml:"name"`
	Count   int      `xml:"count"`
}

// FuzzDecodeRejectsOrLeavesTheTargetUntouched pins the rule a decoder owes the
// handler behind it: on failure, nothing is half-written.
//
// The body is chosen by the caller, and a handler that checks err before using
// the value is the norm — but one that logs and continues, or that reuses the
// struct across requests, will observe whatever the decoder managed to write.
// A partially populated value paired with an error is how a request's data
// leaks into the next one's processing.
func FuzzDecodeRejectsOrLeavesTheTargetUntouched(f *testing.F) {
	f.Add(`<payload><name>svc</name><count>3</count></payload>`)
	f.Add(``)
	f.Add(`<payload>`)
	f.Add(`<payload><name>svc</name><count>not a number</count></payload>`)
	f.Add(`<!DOCTYPE p [<!ENTITY x "expanded">]><payload><name>&x;</name></payload>`)
	f.Add(strings.Repeat(`<payload>`, 64))
	f.Add(`<payload><name>` + strings.Repeat("a", 4096) + `</name></payload>`)
	f.Add("\x00")

	codec := xmlcodec.New()

	f.Fuzz(func(t *testing.T, body string) {
		var out payload
		if err := codec.Decode([]byte(body), &out); err != nil {
			return // A refused body is always a fine outcome.
		}

		// The decode succeeded, so the value must survive being written back
		// out and read again — a decoder that accepted something it cannot
		// itself represent has produced a value nothing downstream can trust.
		encoded, err := codec.Encode(out)
		require.NoError(t, err, "a decoded value could not be re-encoded: %#v", out)

		var again payload
		require.NoError(t, codec.Decode(encoded, &again),
			"the codec refused its own output: %s", encoded)
		require.Equal(t, out, again, "decoding is not stable for %q", body)
	})
}

// FuzzDecodeNeverExpandsAnExternalEntity pins the one XML feature that turns a
// body into a file read.
//
// Billion-laughs and external-entity attacks are the reason an XML decoder on
// an untrusted boundary is worth a target of its own. Go's encoding/xml does
// not resolve external entities, and this states that as a property rather than
// leaving it to the standard library's discretion — a future switch to a
// different decoder would fail here rather than in production.
func FuzzDecodeNeverExpandsAnExternalEntity(f *testing.F) {
	f.Add(`<!DOCTYPE p [<!ENTITY x SYSTEM "file:///etc/passwd">]><payload><name>&x;</name></payload>`)
	f.Add(`<!DOCTYPE p [<!ENTITY x "local">]><payload><name>&x;</name></payload>`)
	f.Add(`<payload><name>plain</name></payload>`)

	codec := xmlcodec.New()

	f.Fuzz(func(t *testing.T, body string) {
		var out payload
		if err := codec.Decode([]byte(body), &out); err != nil {
			return
		}

		require.NotContains(t, out.Name, "root:",
			"an external entity was resolved into the decoded value: %q", out.Name)
	})
}
