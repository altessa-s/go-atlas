// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cors_test

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/cors"
)

// allowedOrigin is the one origin the targets configure. Everything else is a
// foreign origin whose only way into a response header would be a bug.
const allowedOrigin = "https://app.example.com"

// originSeeds are the shapes an attacker reaches for when probing an origin
// check: near-misses on the allowed host, and the values that mean "anything".
var originSeeds = []string{
	allowedOrigin,
	"https://evil.example.com",
	"https://app.example.com.evil.com",
	"https://app.example.com:8443",
	"http://app.example.com",
	"null",
	"*",
	"",
	"https://APP.EXAMPLE.COM",
	"https://app.example.com/",
	"https://app.example.com\r\nX-Injected: 1",
}

// FuzzNeverReflectsAForeignOrigin is the origin-reflection oracle.
//
// Access-Control-Allow-Origin is the browser's entire basis for letting one
// site read another's response. A middleware that echoes back whatever Origin
// arrived turns every configured allowlist into decoration — and paired with
// Access-Control-Allow-Credentials, into a same-origin bypass for any site the
// victim visits.
//
// The property has no exceptions worth carving out: an origin the configuration
// does not name must not appear in the response, on a preflight or on an actual
// request.
func FuzzNeverReflectsAForeignOrigin(f *testing.F) {
	for _, seed := range originSeeds {
		f.Add(seed, true)
		f.Add(seed, false)
	}

	handler := cors.New(
		cors.WithAllowedOrigins(allowedOrigin),
		cors.WithAllowedMethods(http.MethodGet, http.MethodPost),
		cors.WithAllowCredentials(),
	).Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))

	f.Fuzz(func(t *testing.T, origin string, preflight bool) {
		method := http.MethodGet
		if preflight {
			method = http.MethodOptions
		}

		req := httptest.NewRequestWithContext(t.Context(), method, "/resource", http.NoBody)
		req.Header.Set("Origin", origin)
		if preflight {
			req.Header.Set("Access-Control-Request-Method", http.MethodGet)
		}

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		allowed := rec.Header().Get("Access-Control-Allow-Origin")
		if allowed == "" {
			return // Nothing granted, which is the correct answer for a foreign origin.
		}

		require.Equal(t, allowedOrigin, allowed,
			"an origin outside the allowlist was reflected: %q", origin)
	})
}

// FuzzCredentialedResponsesNeverUseAWildcard pins the rule browsers enforce and
// servers get wrong: "*" and credentials are mutually exclusive.
//
// A response combining them is rejected by the browser, so the failure is not a
// breach on its own — but a middleware that emits it is one that decided the
// origin did not matter, which is the state just before a real reflection bug.
func FuzzCredentialedResponsesNeverUseAWildcard(f *testing.F) {
	for _, seed := range originSeeds {
		f.Add(seed)
	}

	handler := cors.New(
		cors.WithAllowedOriginPatterns(regexp.MustCompile(`^https://[a-z]+\.example\.com$`)),
		cors.WithAllowedMethods(http.MethodGet),
		cors.WithAllowCredentials(),
	).Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))

	f.Fuzz(func(t *testing.T, origin string) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/resource", http.NoBody)
		req.Header.Set("Origin", origin)

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		allowed := rec.Header().Get("Access-Control-Allow-Origin")
		if allowed == "" {
			return
		}

		require.NotEqual(t, "*", allowed,
			"a credentialed response used a wildcard origin (request origin %q)", origin)
		require.Equal(t, "true", rec.Header().Get("Access-Control-Allow-Credentials"))

		// Whatever was granted must be something the pattern actually accepts.
		require.Regexp(t, `^https://[a-z]+\.example\.com$`, allowed,
			"a granted origin does not match the configured pattern: %q", allowed)
	})
}

// FuzzGrantedOriginIsHeaderSafe pins that nothing reaching a response header can
// carry the bytes that end one.
//
// The Origin arrives from the client and is written back verbatim when allowed.
// Go's net/http refuses to write a header value containing CR or LF, so a
// response-splitting payload does not escape here — but only as long as the
// value that reaches Set is the vetted one. This target says so explicitly
// rather than leaving it to the standard library's discretion.
func FuzzGrantedOriginIsHeaderSafe(f *testing.F) {
	for _, seed := range originSeeds {
		f.Add(seed)
	}
	f.Add("https://app.example.com\nSet-Cookie: a=b")
	f.Add("https://app.example.com\r\n\r\n<html>")

	handler := cors.New(
		cors.WithAllowedOrigins(allowedOrigin),
		cors.WithAllowedMethods(http.MethodGet),
	).Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))

	f.Fuzz(func(t *testing.T, origin string) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/resource", http.NoBody)
		req.Header.Set("Origin", origin)

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		for _, value := range rec.Header().Values("Access-Control-Allow-Origin") {
			require.NotContains(t, value, "\r", "a CR reached a response header: %q", value)
			require.NotContains(t, value, "\n", "an LF reached a response header: %q", value)
		}
	})
}
