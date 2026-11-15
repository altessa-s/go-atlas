// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-net/clientip"
	"github.com/altessa-s/go-atlas/transport/policy/geoacl"

	httpconfig "github.com/altessa-s/go-atlas/config/http"
	middlewareconfig "github.com/altessa-s/go-atlas/config/middleware"
)

// unplacedGeo answers like a GeoIP database asked about a private address: no
// location, no error.
type unplacedGeo struct{}

func (unplacedGeo) Resolve(context.Context, netip.Addr) (geoacl.GeoInfo, error) {
	return geoacl.GeoInfo{}, nil
}

// geoACL.allowPrivateAddresses reaches the middleware: without it an allow
// list turns an in-cluster caller away.
func TestGeoAclMiddleware_AllowPrivateAddressesFromConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		allow bool
		ip    string
		want  int
	}{
		{"off, private client", false, "10.4.2.9", http.StatusForbidden},
		{"on, private client", true, "10.4.2.9", http.StatusOK},
		{"on, loopback client", true, "127.0.0.1", http.StatusOK},
		{"on, public client", true, "8.8.8.8", http.StatusForbidden},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := httpconfig.DefaultGeoACLMiddleware()
			cfg.Enabled = true
			cfg.DefaultRule = &middlewareconfig.GeoACLRule{AllowCountries: []string{"DE"}}
			cfg.AllowPrivateAddresses = tc.allow

			b := New(&httpconfig.Config{Middlewares: &httpconfig.Middlewares{GeoAcl: &cfg}}).
				UseGeoResolver(unplacedGeo{}).
				WithGeoAclMiddleware()
			require.Empty(t, b.errs)
			require.Len(t, b.configMW, 1)

			h := b.configMW[0].Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			req := httptest.NewRequestWithContext(
				clientip.NewContext(t.Context(), netip.MustParseAddr(tc.ip)), http.MethodGet, "/api/orders", http.NoBody)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, tc.want, rec.Code)
		})
	}
}
