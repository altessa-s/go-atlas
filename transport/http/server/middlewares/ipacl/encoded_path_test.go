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
	"github.com/altessa-s/go-atlas/transport/http/server/router/std"
	"github.com/altessa-s/go-atlas/transport/policy/ipacl"
)

// net/http.ServeMux matches the escaped path, the rules and the ignore list
// the decoded one. An encoded slash used to let a request take the ignore
// entry or miss the deny rule of another endpoint and still be dispatched.
func TestMiddleware_EncodedSlashIsRefused(t *testing.T) {
	t.Parallel()

	client := netip.MustParseAddr("203.0.113.7")
	denyClient := &ipacl.AccessRule{Denylist: []netip.Prefix{netip.PrefixFrom(client, 32)}}

	tests := []struct {
		name   string
		policy ipacl.Policy
		target string
		want   int
	}{
		{"ignored path", ipacl.PolicyDeny, "/v1/healthz", http.StatusOK},
		{"encoded slash borrows the ignore entry", ipacl.PolicyDeny, "/v1%2Fhealthz", http.StatusForbidden},
		{"lower-case encoded slash", ipacl.PolicyDeny, "/v1%2fhealthz", http.StatusForbidden},
		{"denied user path", ipacl.PolicyAllow, "/users/alice", http.StatusForbidden},
		{"encoded slash evades the deny pattern", ipacl.PolicyAllow, "/users/alice%2Fadmin", http.StatusForbidden},
		{"encoded dot segment", ipacl.PolicyAllow, "/users/%2E%2E", http.StatusForbidden},
		{"harmless escape", ipacl.PolicyAllow, "/items/caf%C3%A9", http.StatusOK},
		{"encoded backslash is an ordinary character", ipacl.PolicyAllow, "/items/a%5Cb", http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			reg := ipacl.NewRegistry(tc.policy)
			reg.RegisterPattern(regexp.MustCompile(`/users/[^/]+`), denyClient)

			rt := std.New()
			rt.Use(Middleware(reg, WithIgnorePaths("/v1/healthz")))
			for _, pattern := range []string{"/v1/healthz", "/{tenant}", "/users/{id}", "/items/{id}"} {
				rt.HandleFunc(pattern, func(http.ResponseWriter, *http.Request) {})
			}

			req := httptest.NewRequest(http.MethodGet, tc.target, nil)
			req = req.WithContext(clientip.NewContext(req.Context(), client))
			rec := httptest.NewRecorder()
			rt.ServeHTTP(rec, req)

			require.Equal(t, tc.want, rec.Code)
		})
	}
}
