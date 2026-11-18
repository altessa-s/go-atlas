// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/types/redacted"

	httpclient "github.com/altessa-s/go-atlas-net/http/client"
	netproxy "github.com/altessa-s/go-atlas-net/proxyconfig"
	observabilityconfig "github.com/altessa-s/go-atlas/config/observability"
	proxyconfig "github.com/altessa-s/go-atlas/config/proxy"
)

// White-box: the client the exporter sends with is exercised directly, so a
// refused destination is observed as the dial error instead of a dropped
// export.
func TestNewOTLPHTTPClient_AllowsOnlyTheConfiguredEndpoint(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	require.NoError(t, err)

	for _, tc := range []struct {
		name     string
		endpoint string
		get      string
		wantErr  error
	}{
		{name: "loopback collector by address", endpoint: u.Host, get: srv.URL + "/v1/traces"},
		{name: "loopback collector by name", endpoint: "localhost:" + u.Port(), get: "http://localhost:" + u.Port() + "/v1/traces"},
		{name: "other loopback port", endpoint: u.Host, get: "http://127.0.0.1:1/v1/traces", wantErr: httpclient.ErrSSRFBlocked},
		{name: "cloud metadata", endpoint: u.Host, get: "http://169.254.169.254/latest/meta-data/", wantErr: httpclient.ErrSSRFBlocked},
		{name: "named collector on another port", endpoint: "localhost:" + u.Port(), get: "http://localhost:1/", wantErr: httpclient.ErrSSRFBlocked},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client, err := newOTLPHTTPClient(&observabilityconfig.TracingOTLP{
				Endpoint: tc.endpoint,
				Protocol: observabilityconfig.OTLPProtocolHTTP,
				Insecure: true,
			})
			require.NoError(t, err)
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, tc.get, http.NoBody)
			require.NoError(t, err)
			resp, err := client.Do(req)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			require.Equal(t, http.StatusOK, resp.StatusCode)
		})
	}
}

// A configured proxy is allowed alongside the collector, whatever network it
// is on; other private destinations stay refused.
func TestNewOTLPHTTPClient_AllowsTheConfiguredProxy(t *testing.T) {
	t.Parallel()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(proxy.Close)

	client, err := newOTLPHTTPClient(&observabilityconfig.TracingOTLP{
		Endpoint: "collector.invalid:4318",
		Protocol: observabilityconfig.OTLPProtocolHTTP,
		Insecure: true,
		Proxy:    &proxyconfig.Config{Mode: netproxy.ModeURL, URL: redacted.RedactedString(proxy.URL)},
	})
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://collector.invalid:4318/v1/traces", http.NoBody)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	req, err = http.NewRequestWithContext(t.Context(), http.MethodGet, "http://169.254.169.254/", http.NoBody)
	require.NoError(t, err)
	_, err = client.Do(req)
	require.ErrorIs(t, err, httpclient.ErrSSRFBlocked)
}

// The egress policy only refuses private destinations, so a redirect to a
// public host would pass it and replay the export, custom API-key headers
// included. The client must not follow any redirect: the 3xx reaches the
// exporter as a failed export. The redirect here stays on the collector,
// an allowed destination, so only the redirect refusal can stop it.
func TestNewOTLPHTTPClient_DoesNotFollowRedirects(t *testing.T) {
	t.Parallel()
	var followed atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/elsewhere" {
			followed.Add(1)
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Redirect(w, r, "/elsewhere", http.StatusTemporaryRedirect)
	}))
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	require.NoError(t, err)

	client, err := newOTLPHTTPClient(&observabilityconfig.TracingOTLP{
		Endpoint: u.Host,
		Protocol: observabilityconfig.OTLPProtocolHTTP,
		Insecure: true,
	})
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/v1/traces", http.NoBody)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
	require.Zero(t, followed.Load(), "a redirect must not be followed")
}

func TestNewOTLPHTTPClient_RejectsAnInvalidEndpoint(t *testing.T) {
	t.Parallel()
	for _, endpoint := range []string{"", ":4318", "*.example.com:4318", "collector:notaport"} {
		_, err := newOTLPHTTPClient(&observabilityconfig.TracingOTLP{Endpoint: endpoint, Protocol: observabilityconfig.OTLPProtocolHTTP})
		require.Error(t, err, endpoint)
	}
}
