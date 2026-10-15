// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/clientip"

	httpconfig "github.com/altessa-s/go-atlas/config/http"
)

// White-box access is needed to exercise the configured middleware without starting a server.
func TestRealIPMiddlewareTrust(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		legacy  bool
		proxies []string
		want    string
	}{
		{name: "default", want: "10.0.0.1"},
		{name: "explicit proxy", proxies: []string{"10.0.0.0/8"}, want: "8.8.8.8"},
		{name: "legacy opt out", legacy: true, want: "8.8.8.8"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := httpconfig.DefaultRealIPMiddleware()
			cfg.Enabled = true
			cfg.TrustPrivatePeers = tc.legacy
			cfg.TrustedProxies = tc.proxies
			b := New(&httpconfig.Config{Middlewares: &httpconfig.Middlewares{RealIp: &cfg}}).WithRealIPMiddleware()
			require.Empty(t, b.errs)
			require.Len(t, b.configMW, 1)
			r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
			r.RemoteAddr = "10.0.0.1:1234"
			r.Header.Set("X-Forwarded-For", "8.8.8.8")
			var got string
			b.configMW[0].Handler(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				got = clientip.FromContext(r.Context()).String()
			})).ServeHTTP(httptest.NewRecorder(), r)
			require.Equal(t, tc.want, got)
		})
	}
}
