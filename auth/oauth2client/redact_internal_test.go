// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// White-box: the summary of an IdP error response is not exported. A
// description is always withheld because it may contain credentials.
func TestErrorSummary(t *testing.T) {
	t.Parallel()

	withheld := []struct{ name, description string }{
		{"name=value", "bad client_secret=s3cr3t"},
		{"percent-encoded pair", "bad client_secret%3Ds3cr3t"},
		{"percent-encoded bearer", "rejected Bearer%20eyJhbGciOiJIUzI1NiJ9.e30.c2ln"},
		{"single-quoted pair", "got 'client_secret': 's3cr3t'"},
		{"double-quoted pair", `got "client_secret":"s3cr3t"`},
		{"underscore-prefixed secret", "api_secret=s3cr3t"},
		{"underscore-prefixed token", "oauth_token: rt-123"},
		{"spaced name", "client secret s3cr3t is wrong"},
		{"mixed case", "PassWord hunter2 rejected"},
		{"bare jwt", "eyJhbGciOiJIUzI1NiJ9.e30.c2ln"},
		{"authorization code", "code SplxlOBeZQQYbYS6WxSbIA expired"},
		{"plus-encoded", "client+secret+s3cr3t"},
		{"control characters", "line one\nline two"},
		{"too long", strings.Repeat("a", 257)},
	}
	for _, tc := range withheld {
		t.Run("withheld/"+tc.name, func(t *testing.T) {
			t.Parallel()
			got := errorSummary("invalid_client", tc.description, "", 0)
			require.Regexp(t, `^error "invalid_client" \(description: \d+ bytes withheld\)$`, got)
		})
	}

	t.Run("plain description is withheld", func(t *testing.T) {
		t.Parallel()
		const description = "The requested scope is not allowed for this client (see docs/scopes)."
		require.Equal(t, fmt.Sprintf(`error "invalid_scope" (description: %d bytes withheld)`, len(description)), errorSummary("invalid_scope", description, "", 0))
	})

	t.Run("unusable error codes fall back to the body length", func(t *testing.T) {
		t.Parallel()
		for _, code := range []string{"", "client_secret=s3cr3t", `a"b`, "with space", strings.Repeat("x", 65)} {
			require.Equal(t, "response body of 42 bytes", errorSummary(code, "anything", "", 42), code)
		}
	})

	t.Run("no description", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, `error "invalid_grant"`, errorSummary("invalid_grant", "", "", 0))
	})
}
