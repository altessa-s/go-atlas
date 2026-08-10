// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package writer

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// acceptSeeds are Accept headers worth starting from: ordinary negotiation, the
// wildcards, and the malformed values a client can send for free.
var acceptSeeds = []string{
	"application/json",
	"application/xml",
	"*/*",
	"application/*",
	"",
	"text/xml",
	"application/json;q=0.1, application/xml;q=0.9",
	"application/json;q=notanumber",
	",,,",
	"application/json, */*;q=0",
	strings.Repeat("application/json,", 64),
}

// FuzzWriteDeclaresWhatItWrote is the negotiation oracle for the response side.
//
// The Accept header is caller-controlled and picks the serializer. A response
// whose Content-Type does not describe its body is one a client parses as the
// wrong format — and a body written with no Content-Type at all invites the
// browser to sniff it, which is how a JSON error page becomes executable
// content.
func FuzzWriteDeclaresWhatItWrote(f *testing.F) {
	for _, seed := range acceptSeeds {
		f.Add(seed)
	}

	w := New()

	f.Fuzz(func(t *testing.T, accept string) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
		if accept != "" {
			req.Header.Set("Accept", accept)
		}

		if err := w.Write(rec, req, map[string]string{"key": "value"}); err != nil {
			return // A refused negotiation writes nothing to declare.
		}

		body := rec.Body.String()
		if body == "" {
			return
		}

		require.NotEmpty(t, rec.Header().Get("Content-Type"),
			"a body was written with no Content-Type, leaving the client to sniff it: accept=%q body=%q",
			accept, body)
	})
}

// FuzzWriteIsDeterministic pins that one Accept header always produces one
// encoding.
//
// Two identical requests answered in different formats is the failure a cache
// in front of the service turns into a client receiving XML where it asked for
// JSON — and the Vary header cannot help when the variation is not driven by
// the header at all.
func FuzzWriteIsDeterministic(f *testing.F) {
	for _, seed := range acceptSeeds {
		f.Add(seed)
	}

	w := New()

	write := func(t *testing.T, accept string) (string, string, error) {
		t.Helper()

		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
		if accept != "" {
			req.Header.Set("Accept", accept)
		}

		err := w.Write(rec, req, map[string]string{"key": "value"})
		return rec.Header().Get("Content-Type"), rec.Body.String(), err
	}

	f.Fuzz(func(t *testing.T, accept string) {
		firstType, firstBody, firstErr := write(t, accept)
		secondType, secondBody, secondErr := write(t, accept)

		require.Equal(t, firstErr == nil, secondErr == nil,
			"the same Accept header succeeded once and failed once: %q", accept)
		require.Equal(t, firstType, secondType,
			"the same Accept header produced two content types: %q", accept)
		require.Equal(t, firstBody, secondBody,
			"the same Accept header produced two bodies: %q", accept)
	})
}
