// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	corehttp "github.com/altessa-s/go-atlas/core/net/http"
)

// newProxyServer returns an httptest server that behaves like a forward
// HTTP proxy: it captures the target URL.Host and Proxy-Authorization
// header of every request and replies 204 No Content. Captured values
// are exposed via the returned channel pointers.
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

func TestWithProxyURL_RoutesThroughProxy(t *testing.T) {
	t.Parallel()

	proxy, lastHost, _ := newProxyServer(t)
	proxyURL, err := url.Parse(proxy.URL)
	require.NoError(t, err)

	c := New(WithProxyURL(proxyURL), WithRetryMax(0))
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.invalid/path", nil)
	require.NoError(t, err)

	resp, err := c.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.Equal(t, "example.invalid", lastHost.Load())
}

func TestWithProxyFunc_DynamicRouting(t *testing.T) {
	t.Parallel()

	proxyA, hostA, _ := newProxyServer(t)
	proxyB, hostB, _ := newProxyServer(t)

	urlA, err := url.Parse(proxyA.URL)
	require.NoError(t, err)
	urlB, err := url.Parse(proxyB.URL)
	require.NoError(t, err)

	resolver := func(req *http.Request) (*url.URL, error) {
		if strings.HasPrefix(req.URL.Host, "a.") {
			return urlA, nil
		}
		return urlB, nil
	}

	c := New(WithProxyFunc(resolver), WithRetryMax(0))

	for _, target := range []string{"http://a.example.invalid/x", "http://b.example.invalid/y"} {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)
		require.NoError(t, err)
		resp, err := c.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
	}

	require.Equal(t, "a.example.invalid", hostA.Load())
	require.Equal(t, "b.example.invalid", hostB.Load())
}

func TestWithoutProxy_OverridesEarlierOption(t *testing.T) {
	t.Parallel()

	// Stand up a proxy and immediately close it: any request that goes
	// through it will fail. WithoutProxy must override the earlier
	// WithProxyURL so the request reaches target directly.
	deadProxy := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	deadProxyURL, err := url.Parse(deadProxy.URL)
	require.NoError(t, err)
	deadProxy.Close()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(target.Close)

	c := New(WithProxyURL(deadProxyURL), WithoutProxy(), WithRetryMax(0))
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, target.URL, nil)
	require.NoError(t, err)
	resp, err := c.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestWithProxy_DoesNotMutateProvidedTransport(t *testing.T) {
	t.Parallel()

	proxy, _, _ := newProxyServer(t)
	proxyURL, err := url.Parse(proxy.URL)
	require.NoError(t, err)

	provided := &http.Transport{}
	require.Nil(t, provided.Proxy, "precondition")

	_ = New(WithTransport(provided), WithProxyURL(proxyURL))

	require.Nil(t, provided.Proxy, "WithProxyURL mutated caller-supplied transport")
}

func TestWithProxy_DoesNotMutateProvidedClient(t *testing.T) {
	t.Parallel()

	proxy, lastHost, _ := newProxyServer(t)
	proxyURL, err := url.Parse(proxy.URL)
	require.NoError(t, err)

	origTransport := &http.Transport{}
	custom := &http.Client{Transport: origTransport}

	c := New(WithClient(custom), WithProxyURL(proxyURL), WithRetryMax(0))
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.invalid/", nil)
	require.NoError(t, err)
	resp, err := c.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, "example.invalid", lastHost.Load())
	require.Same(t, origTransport, custom.Transport, "WithClient *http.Client.Transport was swapped")
	require.Nil(t, origTransport.Proxy, "WithClient transport's Proxy field was mutated")
}

func TestWithSSRFProtection_DoesNotMutateProvidedClient(t *testing.T) {
	t.Parallel()

	// SSRF protection wraps the underlying *http.Transport. When the
	// caller supplied an *http.Client via WithClient (and no transport
	// override / proxy was set), the wrap must clone the client rather
	// than mutate Transport in place — otherwise other consumers
	// sharing that *http.Client would silently route through the
	// SSRF-safe transport too. Mirror of TestWithProxy_DoesNotMutateProvidedClient.
	origTransport := &http.Transport{}
	custom := &http.Client{Transport: origTransport}

	_ = New(WithClient(custom), WithSSRFProtection(), WithRetryMax(0))

	require.Same(t, origTransport, custom.Transport,
		"WithSSRFProtection mutated caller-supplied *http.Client.Transport")
}

func TestWithProxy_NilTransportClient_AppliesResolver(t *testing.T) {
	t.Parallel()

	// A *http.Client with nil Transport is legal — net/http falls back to
	// http.DefaultTransport at request time. Without nil-normalisation
	// the proxy block silently no-ops, and the request would slip through
	// http.DefaultTransport.Proxy (== http.ProxyFromEnvironment), defeating
	// WithoutProxy and WithProxy*.
	proxy, lastHost, _ := newProxyServer(t)
	proxyURL, err := url.Parse(proxy.URL)
	require.NoError(t, err)

	c := New(WithClient(&http.Client{}), WithProxyURL(proxyURL), WithRetryMax(0))
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.invalid/", nil)
	require.NoError(t, err)
	resp, err := c.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, "example.invalid", lastHost.Load(), "proxy resolver was not consulted — nil Transport not normalised")
}

func TestWithProxy_NonTransportRoundTripper_NoOp(t *testing.T) {
	t.Parallel()

	var called atomic.Bool
	rt := corehttp.RoundTripperFunc(func(*http.Request) (*http.Response, error) {
		called.Store(true)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}, nil
	})

	custom := &http.Client{Transport: rt}
	proxyURL, _ := url.Parse("http://127.0.0.1:1")

	c := New(WithClient(custom), WithProxyURL(proxyURL), WithRetryMax(0))
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.invalid/", nil)
	require.NoError(t, err)
	resp, err := c.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.True(t, called.Load(), "custom RoundTripper was bypassed")
}

func TestDefaultProxyBehavior_Unchanged(t *testing.T) {
	t.Parallel()

	require.Nil(t, newOptions().proxy, "fresh options must not carry a proxy resolver")

	tr := defaultPooledTransport()
	require.NotNil(t, tr.Proxy, "default transport lost its env-based proxy resolver")

	want := reflect.ValueOf(http.ProxyFromEnvironment).Pointer()
	got := reflect.ValueOf(tr.Proxy).Pointer()
	require.Equal(t, want, got, "default transport.Proxy is no longer http.ProxyFromEnvironment")
}

func TestWithProxy_HostPortAuth(t *testing.T) {
	t.Parallel()

	t.Run("valid", func(t *testing.T) {
		t.Parallel()

		valid := []struct {
			name string
			auth *url.Userinfo
			// wantAuth is the expected Proxy-Authorization header value
			// (empty string means the header must be absent).
			wantAuth string
		}{
			{name: "anonymous", auth: nil},
			{name: "user_password", auth: url.UserPassword("svc", "secret"), wantAuth: "Basic c3ZjOnNlY3JldA=="},
			{name: "user_only", auth: url.User("svc"), wantAuth: "Basic c3ZjOg=="},
		}
		for _, tc := range valid {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				proxy, _, lastAuth := newProxyServer(t)
				proxyURL, err := url.Parse(proxy.URL)
				require.NoError(t, err)
				port, err := strconv.Atoi(proxyURL.Port())
				require.NoError(t, err)

				c := New(WithProxy(proxyURL.Hostname(), port, tc.auth), WithRetryMax(0))
				req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.invalid/", nil)
				require.NoError(t, err)
				resp, err := c.Do(req)
				require.NoError(t, err)
				defer resp.Body.Close()
				require.Equal(t, tc.wantAuth, lastAuth.Load())
			})
		}
	})

	t.Run("invalid", func(t *testing.T) {
		t.Parallel()

		invalid := []struct {
			name string
			host string
			port int
		}{
			{name: "empty_host", host: "", port: 3128},
			{name: "port_zero", host: "proxy", port: 0},
			{name: "port_too_large", host: "proxy", port: 70000},
		}
		for _, tc := range invalid {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				c := New(WithProxy(tc.host, tc.port, nil), WithRetryMax(0))
				req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.invalid/", nil)
				require.NoError(t, err)
				_, err = c.Do(req)
				require.Error(t, err)
			})
		}
	})
}

func TestWithProxyURL_Nil_FailsAtRequestTime(t *testing.T) {
	t.Parallel()

	c := New(WithProxyURL(nil), WithRetryMax(0))
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.invalid/", nil)
	require.NoError(t, err)
	_, err = c.Do(req)
	require.Error(t, err)
}

func TestWithProxyTLSConfig_StoresOption(t *testing.T) {
	t.Parallel()

	pool := x509.NewCertPool()
	cfg := &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pool, ServerName: "user-set.example"}

	o := defaultOptions()
	WithProxyTLSConfig(cfg)(o)
	require.Same(t, cfg, o.proxyTLSConfig)
	// Merge / clone semantics live in transport/internal/proxydial and
	// are exercised in proxydial_test.go — the test here only verifies
	// the option-storage hook on options.
}
