// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/transport/proxydial/factory"

	httpclient "github.com/altessa-s/go-atlas/transport/http/client"
)

func TestProxy_ClientOptions_NilReceiver(t *testing.T) {
	t.Parallel()
	var p *config.Proxy

	httpOpts, err := factory.HTTPClientOptions(p)
	require.NoError(t, err)
	require.Nil(t, httpOpts)

	grpcOpts, err := factory.GRPCClientOptions(p)
	require.NoError(t, err)
	require.Nil(t, grpcOpts)
}

func TestProxy_ClientOptions_PassthroughReturnsNil(t *testing.T) {
	t.Parallel()

	// Empty Mode == "no override" — leave the underlying transport's
	// env-based default in place (http.ProxyFromEnvironment for
	// net/http, grpc-go's own HTTPS_PROXY lookup for gRPC).
	cfg := config.Proxy{}

	httpOpts, err := factory.HTTPClientOptions(&cfg)
	require.NoError(t, err)
	require.Nil(t, httpOpts)

	grpcOpts, err := factory.GRPCClientOptions(&cfg)
	require.NoError(t, err)
	require.Nil(t, grpcOpts)
}

func TestProxy_ClientOptions_UnknownMode(t *testing.T) {
	t.Parallel()
	cfg := config.Proxy{Mode: "bogus"}

	httpOpts, err := factory.HTTPClientOptions(&cfg)
	require.Error(t, err)
	require.Nil(t, httpOpts)

	grpcOpts, err := factory.GRPCClientOptions(&cfg)
	require.Error(t, err)
	require.Nil(t, grpcOpts)
}

func TestProxy_ClientOptions_URLParseError(t *testing.T) {
	t.Parallel()
	cfg := config.Proxy{Mode: config.ProxyModeURL, URL: "::bad"}

	httpOpts, err := factory.HTTPClientOptions(&cfg)
	require.Error(t, err)
	require.Nil(t, httpOpts)

	grpcOpts, err := factory.GRPCClientOptions(&cfg)
	require.Error(t, err)
	require.Nil(t, grpcOpts)
}

func TestProxy_GrpcClientOptions_NonEnvReturnsOption(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cfg  config.Proxy
	}{
		{"none", config.Proxy{Mode: config.ProxyModeNone}},
		{"url", config.Proxy{Mode: config.ProxyModeURL, URL: "http://proxy:3128"}},
		{"host_no_auth", config.Proxy{Mode: config.ProxyModeHost, Host: "proxy", Port: 3128}},
		{"host_with_auth", config.Proxy{
			Mode: config.ProxyModeHost, Host: "proxy", Port: 3128,
			Auth: &config.ProxyAuth{Username: "svc", Password: "secret"},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			opts, err := factory.GRPCClientOptions(&tc.cfg)
			require.NoError(t, err)
			require.Len(t, opts, 1)
		})
	}
}

// TestProxy_HTTPClientOptions_RoutesThroughProxy spins up an httptest
// server in proxy role and checks that the materialized options actually
// route requests through it for every non-trivial mode.

func TestProxy_HTTPClientOptions_RoutesThroughProxy(t *testing.T) {
	t.Parallel()

	t.Run("mode_url", func(t *testing.T) {
		t.Parallel()
		proxy, lastHost, _ := newProxyServer(t)
		cfg := config.Proxy{Mode: config.ProxyModeURL, URL: proxy.URL}
		runProxiedGet(t, cfg)
		require.Equal(t, "example.invalid", lastHost.Load())
	})

	t.Run("mode_host_no_auth", func(t *testing.T) {
		t.Parallel()
		proxy, lastHost, lastAuth := newProxyServer(t)
		host, port := splitProxyURL(t, proxy.URL)
		cfg := config.Proxy{Mode: config.ProxyModeHost, Host: host, Port: port}
		runProxiedGet(t, cfg)
		require.Equal(t, "example.invalid", lastHost.Load())
		require.Empty(t, lastAuth.Load())
	})

	t.Run("mode_host_with_auth", func(t *testing.T) {
		t.Parallel()
		proxy, lastHost, lastAuth := newProxyServer(t)
		host, port := splitProxyURL(t, proxy.URL)
		cfg := config.Proxy{
			Mode: config.ProxyModeHost, Host: host, Port: port,
			Auth: &config.ProxyAuth{Username: "svc", Password: "secret"},
		}
		runProxiedGet(t, cfg)
		require.Equal(t, "example.invalid", lastHost.Load())
		// "Basic c3ZjOnNlY3JldA==" == base64("svc:secret")
		require.Equal(t, "Basic c3ZjOnNlY3JldA==", lastAuth.Load())
	})

	t.Run("mode_host_username_only", func(t *testing.T) {
		t.Parallel()
		proxy, _, lastAuth := newProxyServer(t)
		host, port := splitProxyURL(t, proxy.URL)
		cfg := config.Proxy{
			Mode: config.ProxyModeHost, Host: host, Port: port,
			Auth: &config.ProxyAuth{Username: "svc"},
		}
		runProxiedGet(t, cfg)
		// "Basic c3ZjOg==" == base64("svc:")
		require.Equal(t, "Basic c3ZjOg==", lastAuth.Load())
	})

	t.Run("mode_none_disables_proxy", func(t *testing.T) {
		t.Parallel()
		// A target server reachable directly. With Mode=none there is
		// no proxy, so the request must arrive at the target itself.
		hit := atomic.Bool{}
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			hit.Store(true)
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(target.Close)

		cfg := config.Proxy{Mode: config.ProxyModeNone}
		opts, err := factory.HTTPClientOptions(&cfg)
		require.NoError(t, err)
		require.Len(t, opts, 1)

		c := httpclient.New(append(opts, httpclient.WithRetryMax(0))...)
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, target.URL, nil)
		require.NoError(t, err)
		resp, err := c.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.True(t, hit.Load())
	})
}

func newProxyServer(t *testing.T) (srv *httptest.Server, lastHost, lastAuth *atomic.Value) {
	t.Helper()
	lastHost = &atomic.Value{}
	lastAuth = &atomic.Value{}
	lastHost.Store("")
	lastAuth.Store("")
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastHost.Store(r.URL.Host)
		lastAuth.Store(r.Header.Get("Proxy-Authorization"))
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	return srv, lastHost, lastAuth
}

// splitProxyURL parses an httptest server URL into (host, port).

func splitProxyURL(t *testing.T, raw string) (string, int) {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	host := u.Hostname()
	port := 0
	for _, r := range u.Port() {
		require.True(t, r >= '0' && r <= '9')
		port = port*10 + int(r-'0')
	}
	return host, port
}

// runProxiedGet builds an httpclient.Client from cfg and issues a single
// GET to a non-resolvable host. If the proxy is wired correctly the
// request reaches the proxy server (which always replies 204) and the
// caller can inspect the captured proxy state via the returned values.

func runProxiedGet(t *testing.T, cfg config.Proxy) {
	t.Helper()
	opts, err := factory.HTTPClientOptions(&cfg)
	require.NoError(t, err)

	c := httpclient.New(append(opts, httpclient.WithRetryMax(0))...)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
		"http://example.invalid/path", nil)
	require.NoError(t, err)
	resp, err := c.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
}
