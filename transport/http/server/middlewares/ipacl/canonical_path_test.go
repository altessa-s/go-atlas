// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package ipacl

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-net/clientip"
	"github.com/altessa-s/go-atlas/transport/policy/ipacl"
)

// A spelling of /admin that is not canonical used to miss the /admin rule and
// take the PolicyAllow default; with an ignored prefix it skipped the check.
func TestMiddleware_NonCanonicalPathIsRefused(t *testing.T) {
	t.Parallel()

	client := netip.MustParseAddr("203.0.113.7")
	reg := ipacl.NewRegistry(ipacl.PolicyAllow)
	reg.Register("/admin", &ipacl.AccessRule{Denylist: []netip.Prefix{netip.PrefixFrom(client, 32)}})

	tests := []struct {
		path       string
		wantStatus int
	}{
		{"/public", http.StatusOK},
		{"/static/app.js", http.StatusOK},
		{"/admin", http.StatusForbidden},
		{"//admin", http.StatusForbidden},
		{"/./admin", http.StatusForbidden},
		{"/public/../admin", http.StatusForbidden},
		{"/static/../admin", http.StatusForbidden},
		{"/admin//", http.StatusForbidden},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()
			called := false
			handler := Middleware(reg, WithIgnorePatterns(regexp.MustCompile(`^/static/`)))(
				http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req = req.WithContext(clientip.NewContext(req.Context(), client))
			req.URL.Path = tc.path
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			require.Equal(t, tc.wantStatus, rec.Code)
			require.Equal(t, tc.wantStatus == http.StatusOK, called)
		})
	}
}
