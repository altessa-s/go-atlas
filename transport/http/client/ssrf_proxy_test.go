// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client_test

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	httpclient "github.com/altessa-s/go-atlas/transport/http/client"
)

func TestSSRFDialControl(t *testing.T) {
	t.Parallel()

	allowed10 := netip.MustParsePrefix("10.0.0.0/8")
	allowedMapped := netip.MustParsePrefix("::ffff:192.168.0.0/112")
	tests := []struct {
		name    string
		allowed []netip.Prefix
		address string
		blocked bool
	}{
		{"public v4", nil, "8.8.8.8:443", false},
		{"public v6", nil, "[2001:4860:4860::8888]:443", false},
		{"rfc1918", nil, "10.1.2.3:80", true},
		{"loopback", nil, "127.0.0.1:80", true},
		{"loopback v6", nil, "[::1]:80", true},
		{"metadata", nil, "169.254.169.254:80", true},
		{"cgnat", nil, "100.64.0.1:80", true},
		{"benchmark", nil, "198.18.0.1:80", true},
		{"ula", nil, "[fd00::1]:80", true},
		{"mapped private", nil, "[::ffff:10.0.0.1]:80", true},
		{"mapped loopback", nil, "[::ffff:127.0.0.1]:80", true},
		{"mapped public", nil, "[::ffff:8.8.8.8]:80", false},
		{"zoned link-local", nil, "[fe80::1%en0]:80", true},
		{"allowed range", []netip.Prefix{allowed10}, "10.1.2.3:80", false},
		{"allowed range, mapped address", []netip.Prefix{allowed10}, "[::ffff:10.1.2.3]:80", false},
		{"allowed range, other private", []netip.Prefix{allowed10}, "192.168.1.1:80", true},
		{"mapped allowed range", []netip.Prefix{allowedMapped}, "192.168.1.1:80", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := httpclient.SSRFDialControl(tc.allowed...)("tcp", tc.address, nil)
			if tc.blocked {
				require.ErrorIs(t, err, httpclient.ErrSSRFBlocked)
				require.NotNil(t, httpclient.IsSSRFError(err))
				return
			}
			require.NoError(t, err)
		})
	}

	t.Run("not an ip:port", func(t *testing.T) {
		t.Parallel()

		for _, addr := range []string{"example.com:80", "10.0.0.1", ""} {
			err := httpclient.SSRFDialControl()("tcp", addr, nil)
			require.Error(t, err, addr)
		}
	})

	t.Run("caller slice is not retained", func(t *testing.T) {
		t.Parallel()

		allowed := []netip.Prefix{allowed10}
		ctl := httpclient.SSRFDialControl(allowed...)
		allowed[0] = netip.MustParsePrefix("192.168.0.0/16")
		require.NoError(t, ctl("tcp", "10.0.0.1:80", nil))
		require.ErrorIs(t, ctl("tcp", "192.168.0.1:80", nil), httpclient.ErrSSRFBlocked)
	})
}

// forwardProxy is a plain-HTTP forward proxy stub: it records the absolute
// request URLs it receives and answers 204 without contacting them.
type forwardProxy struct {
	mu   sync.Mutex
	urls []string
}

func (p *forwardProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	p.urls = append(p.urls, r.URL.String())
	p.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (p *forwardProxy) seen() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.urls...)
}

// startProxy serves a forwardProxy on loopback and returns its URL.
func startProxy(t *testing.T) (*forwardProxy, *url.URL) {
	t.Helper()
	p := &forwardProxy{}
	srv := httptest.NewServer(p)
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	require.NoError(t, err)
	return p, u
}

var proxyLoopback = httpclient.WithSSRFAllowedCIDRs(netip.MustParsePrefix("127.0.0.0/8"))

func TestSSRFWithProxy_ConstructionAccepted(t *testing.T) {
	t.Parallel()

	require.NotPanics(t, func() {
		_ = httpclient.New(httpclient.WithProxy("proxy.example", 3128, nil))
		_ = httpclient.New(httpclient.WithProxyFunc(func(*http.Request) (*url.URL, error) {
			panic("the resolver must not run at construction")
		}))
	})
}

func TestSSRFWithProxy_ProxyDialIsChecked(t *testing.T) {
	t.Parallel()

	p, proxyURL := startProxy(t)
	c := httpclient.New(httpclient.WithProxyURL(proxyURL), httpclient.WithRetryMax(0))
	_, err := get(t, c, "http://target.example/path")
	require.ErrorIs(t, err, httpclient.ErrSSRFBlocked, "a proxy on loopback is blocked like any destination")
	require.Empty(t, p.seen())
}

func TestSSRFWithProxy_TargetResolvedByProxy(t *testing.T) {
	t.Parallel()

	p, proxyURL := startProxy(t)
	c := httpclient.New(httpclient.WithProxyURL(proxyURL), proxyLoopback, httpclient.WithRetryMax(0))

	// The host name is not resolved locally: target.invalid has no DNS
	// record, yet the request reaches the proxy, which resolves targets.
	resp, err := get(t, c, "http://target.invalid/path")
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	// A public literal IP passes the literal check.
	resp, err = get(t, c, "http://8.8.8.8/dns")
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	require.Equal(t, []string{"http://target.invalid/path", "http://8.8.8.8/dns"}, p.seen())
}

func TestSSRFWithProxy_PrivateLiteralTargetBlocked(t *testing.T) {
	t.Parallel()

	p, proxyURL := startProxy(t)
	var resolved atomic.Int32
	resolver := func(*http.Request) (*url.URL, error) {
		resolved.Add(1)
		return proxyURL, nil
	}
	c := httpclient.New(httpclient.WithProxyFunc(resolver), proxyLoopback, httpclient.WithRetryMax(0))

	for _, target := range []string{
		"http://169.254.169.254/latest/meta-data",
		"http://10.0.0.1/",
		"http://[::ffff:10.0.0.1]/",
		"http://[fd00::1]/",
	} {
		_, err := get(t, c, target)
		require.ErrorIs(t, err, httpclient.ErrSSRFBlocked, target)
	}
	require.Empty(t, p.seen(), "a blocked literal target never reaches the proxy")
	require.Positive(t, resolved.Load())

	// The proxy network is allowed (loopback); a literal target in an allowed
	// range is not blocked either.
	resp, err := get(t, c, "http://127.0.0.1:9/")
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestSSRFWithProxy_WithoutProxyStillDirect(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	c := httpclient.New(httpclient.WithoutProxy(), proxyLoopback, httpclient.WithRetryMax(0))
	resp, err := get(t, c, srv.URL)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.Equal(t, int32(1), hits.Load())
}
