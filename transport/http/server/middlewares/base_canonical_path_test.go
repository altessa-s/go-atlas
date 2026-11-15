// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package middlewares

import (
	"net/http"
	"net/url"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

// An ignore list is consulted for the canonical path only. A prefix pattern
// such as ^/public/ used to ignore "/public/../admin", which a router then
// cleans to "/admin"; "//healthz" is no spelling of the ignored "/healthz".
func TestBaseMiddleware_ShouldIgnoreRequest_CanonicalPathOnly(t *testing.T) {
	t.Parallel()

	b := NewBaseMiddlewareWithFilter("test",
		[]string{"/healthz", "/metrics/"},
		[]*regexp.Regexp{regexp.MustCompile(`^/public/`), regexp.MustCompile(`healthz`)},
		nil,
	)

	tests := []struct {
		path string
		want bool
	}{
		{"/healthz", true},
		{"/metrics/", true},
		{"/public/logo.png", true},
		{"/public/../admin", false},
		{"/public/./../admin", false},
		{"/public//admin", false},
		{"/healthz/../admin", false},
		{"/admin/../healthz", false},
		{"//healthz", false},
		{"/./healthz", false},
		{"healthz", false},
		{"", false},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, b.ShouldIgnoreRequest(&http.Request{URL: &url.URL{Path: tc.path}}))
		})
	}
}
