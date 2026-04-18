// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	httpclient "github.com/altessa-s/go-atlas/transport/http/client"
)

func TestHTTPProxy_Validate_ValidCases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cfg  HTTPProxy
	}{
		{"empty_passthrough", HTTPProxy{}},
		{"none", HTTPProxy{Mode: HTTPProxyModeNone}},
		{"url_http", HTTPProxy{Mode: HTTPProxyModeURL, URL: "http://proxy:3128"}},
		{"url_https", HTTPProxy{Mode: HTTPProxyModeURL, URL: "https://proxy:3128"}},
		{"url_socks5", HTTPProxy{Mode: HTTPProxyModeURL, URL: "socks5://proxy:1080"}},
		{"url_socks5h", HTTPProxy{Mode: HTTPProxyModeURL, URL: "socks5h://proxy:1080"}},
		{"host_no_auth", HTTPProxy{Mode: HTTPProxyModeHost, Host: "proxy", Port: 3128}},
		{"host_with_auth", HTTPProxy{
			Mode: HTTPProxyModeHost, Host: "proxy", Port: 3128,
			Auth: &HTTPProxyAuth{Username: "svc", Password: "secret"},
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.NoError(t, tc.cfg.Validate())
		})
	}
}

func TestHTTPProxy_Validate_InvalidCases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cfg  HTTPProxy
	}{
		{"unknown_mode", HTTPProxy{Mode: "bogus"}},
		{"env_mode_no_longer_valid", HTTPProxy{Mode: "env"}},
		{"url_mode_missing_url", HTTPProxy{Mode: HTTPProxyModeURL}},
		{"url_mode_invalid_url", HTTPProxy{Mode: HTTPProxyModeURL, URL: "::not-a-url"}},
		{"url_mode_no_port", HTTPProxy{Mode: HTTPProxyModeURL, URL: "http://proxy.corp"}},
		{"host_mode_missing_host", HTTPProxy{Mode: HTTPProxyModeHost, Port: 3128}},
		{"host_mode_missing_port", HTTPProxy{Mode: HTTPProxyModeHost, Host: "proxy"}},
		{"host_mode_port_zero", HTTPProxy{Mode: HTTPProxyModeHost, Host: "proxy", Port: 0}},
		{"host_mode_port_negative", HTTPProxy{Mode: HTTPProxyModeHost, Host: "proxy", Port: -1}},
		{"host_mode_port_too_large", HTTPProxy{Mode: HTTPProxyModeHost, Host: "proxy", Port: 70000}},
		{"url_mode_with_host_field", HTTPProxy{Mode: HTTPProxyModeURL, URL: "http://p:1", Host: "x"}},
		{"empty_mode_with_url_field", HTTPProxy{URL: "http://p:1"}},
		{"none_mode_with_host_field", HTTPProxy{Mode: HTTPProxyModeNone, Host: "x"}},
		{"host_mode_auth_without_username", HTTPProxy{
			Mode: HTTPProxyModeHost, Host: "p", Port: 1,
			Auth: &HTTPProxyAuth{Password: "x"},
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Error(t, tc.cfg.Validate())
		})
	}
}

func TestHTTPProxy_Validate_NilReceiver(t *testing.T) {
	t.Parallel()
	var p *HTTPProxy
	assert.NoError(t, p.Validate())
}

func TestHTTPProxy_ClientOptions_NilReceiver(t *testing.T) {
	t.Parallel()
	var p *HTTPProxy
	opts, err := p.ClientOptions()
	assert.NoError(t, err)
	assert.Nil(t, opts)
}

func TestHTTPProxy_ClientOptions_PassthroughReturnsNil(t *testing.T) {
	t.Parallel()

	// Empty Mode == "no override" — leave the underlying transport's
	// env-based default in place.
	cfg := HTTPProxy{}
	opts, err := cfg.ClientOptions()
	assert.NoError(t, err)
	assert.Nil(t, opts)
}

func TestHTTPProxy_ClientOptions_UnknownMode(t *testing.T) {
	t.Parallel()
	cfg := HTTPProxy{Mode: "bogus"}
	opts, err := cfg.ClientOptions()
	assert.Error(t, err)
	assert.Nil(t, opts)
}

func TestHTTPProxy_ClientOptions_URLParseError(t *testing.T) {
	t.Parallel()
	cfg := HTTPProxy{Mode: HTTPProxyModeURL, URL: "::bad"}
	opts, err := cfg.ClientOptions()
	assert.Error(t, err)
	assert.Nil(t, opts)
}

// TestHTTPProxy_ClientOptions_RoutesThroughProxy spins up an httptest
// server in proxy role and checks that the materialized options actually
// route requests through it for every non-trivial mode.
func TestHTTPProxy_ClientOptions_RoutesThroughProxy(t *testing.T) {
	t.Parallel()

	t.Run("mode_url", func(t *testing.T) {
		t.Parallel()
		proxy, lastHost, _ := newProxyServer(t)
		cfg := HTTPProxy{Mode: HTTPProxyModeURL, URL: proxy.URL}
		runProxiedGet(t, cfg)
		assert.Equal(t, "example.invalid", lastHost.Load())
	})

	t.Run("mode_host_no_auth", func(t *testing.T) {
		t.Parallel()
		proxy, lastHost, lastAuth := newProxyServer(t)
		host, port := splitProxyURL(t, proxy.URL)
		cfg := HTTPProxy{Mode: HTTPProxyModeHost, Host: host, Port: port}
		runProxiedGet(t, cfg)
		assert.Equal(t, "example.invalid", lastHost.Load())
		assert.Empty(t, lastAuth.Load())
	})

	t.Run("mode_host_with_auth", func(t *testing.T) {
		t.Parallel()
		proxy, lastHost, lastAuth := newProxyServer(t)
		host, port := splitProxyURL(t, proxy.URL)
		cfg := HTTPProxy{
			Mode: HTTPProxyModeHost, Host: host, Port: port,
			Auth: &HTTPProxyAuth{Username: "svc", Password: "secret"},
		}
		runProxiedGet(t, cfg)
		assert.Equal(t, "example.invalid", lastHost.Load())
		// "Basic c3ZjOnNlY3JldA==" == base64("svc:secret")
		assert.Equal(t, "Basic c3ZjOnNlY3JldA==", lastAuth.Load())
	})

	t.Run("mode_host_username_only", func(t *testing.T) {
		t.Parallel()
		proxy, _, lastAuth := newProxyServer(t)
		host, port := splitProxyURL(t, proxy.URL)
		cfg := HTTPProxy{
			Mode: HTTPProxyModeHost, Host: host, Port: port,
			Auth: &HTTPProxyAuth{Username: "svc"},
		}
		runProxiedGet(t, cfg)
		// "Basic c3ZjOg==" == base64("svc:")
		assert.Equal(t, "Basic c3ZjOg==", lastAuth.Load())
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

		cfg := HTTPProxy{Mode: HTTPProxyModeNone}
		opts, err := cfg.ClientOptions()
		require.NoError(t, err)
		require.Len(t, opts, 1)

		c := httpclient.New(append(opts, httpclient.WithRetryMax(0))...)
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, target.URL, nil)
		require.NoError(t, err)
		resp, err := c.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.True(t, hit.Load())
	})
}

func TestHTTPProxy_PasswordRedaction(t *testing.T) {
	t.Parallel()

	password := "topsecret-do-not-leak"
	auth := &HTTPProxyAuth{Username: "svc", Password: Secret(password)}

	// Cover every common output sink: fmt %+v on the dereferenced struct
	// (the pointer-print path shows an address, not the fields), and the
	// fmt.Stringer/%v path on Secret directly.
	rendered := fmt.Sprintf("%+v", *auth) + " | " + fmt.Sprintf("%v", auth.Password)
	assert.NotContains(t, rendered, password)
	assert.Contains(t, rendered, redacted)
}

func TestDefaultHTTPProxy(t *testing.T) {
	t.Parallel()

	got := DefaultHTTPProxy()
	assert.Empty(t, string(got.Mode), "default Mode is the empty zero value (passthrough)")
	assert.NoError(t, got.Validate())

	opts, err := got.ClientOptions()
	assert.NoError(t, err)
	assert.Nil(t, opts)
}

// newProxyServer returns an httptest server that behaves like a forward
// HTTP proxy: it captures the target URL.Host and Proxy-Authorization
// header of every request and replies 204 No Content. Captured values
// are exposed via the returned atomic.Value pointers.
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
func runProxiedGet(t *testing.T, cfg HTTPProxy) {
	t.Helper()
	opts, err := cfg.ClientOptions()
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
