// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package middlewares

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

// The ignore list reads the decoded path while net/http.ServeMux matches the
// escaped one: "/v1%2Fhealthz" decodes to the ignored "/v1/healthz" and is
// dispatched as the single segment "v1/healthz". Such a request is never
// ignored.
func TestBaseMiddleware_ShouldIgnoreRequest_EncodedPath(t *testing.T) {
	t.Parallel()

	b := NewBaseMiddlewareWithFilter("test",
		[]string{"/v1/healthz"},
		[]*regexp.Regexp{regexp.MustCompile(`^/public/`)},
		nil,
	)

	tests := []struct {
		target string
		want   bool
	}{
		{"/v1/healthz", true},
		{"/public/logo.png", true},
		{"/public/caf%C3%A9.png", true},
		{"/public/a%5Cb", true},
		{"/v1%2Fhealthz", false},
		{"/v1%2fhealthz", false},
		{"/public%2Flogo.png", false},
		{"/public/x%2F..%2Fadmin", false},
		{"/public/%2E%2E/admin", false},
		{"/public/%2e%2e/admin", false},
		{"/public/../admin", false},
		{"//v1/healthz", false},
	}
	for _, tc := range tests {
		t.Run(tc.target, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, b.ShouldIgnoreRequest(httptest.NewRequest(http.MethodGet, tc.target, nil)))
		})
	}
}
