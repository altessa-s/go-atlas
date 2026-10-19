// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func TestSSRFRejectsUnsafeTransport(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		opts []Option
	}{
		{"transport proxy", []Option{WithTransport(&http.Transport{Proxy: http.ProxyURL(&url.URL{Scheme: "http", Host: "proxy.example"})})}},
		{"TLS dialer", []Option{WithTransport(&http.Transport{DialTLS: func(string, string) (net.Conn, error) { panic("must not dial") }})}},
		{"context TLS dialer", []Option{WithTransport(&http.Transport{DialTLSContext: func(context.Context, string, string) (net.Conn, error) { panic("must not dial") }})}},
		{"proxy TLS", []Option{WithProxyTLSConfig(&tls.Config{MinVersion: tls.VersionTLS12})}},
		{"custom round tripper", []Option{WithClient(&http.Client{Transport: testhelpers.RoundTripFunc(func(*http.Request) (*http.Response, error) { panic("must not send") })})}},
		{"typed nil transport", []Option{WithClient(&http.Client{Transport: (*http.Transport)(nil)})}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var recovered any
			func() { defer func() { recovered = recover() }(); New(tc.opts...) }()
			err, ok := recovered.(error)
			require.True(t, ok, "construction must panic with an error")
			require.ErrorIs(t, err, ErrSSRFUnsafeTransport)
		})
	}
}

func TestSSRFDisablesEnvironmentProxy(t *testing.T) {
	// Environment variables are process-global, so this test is serial.
	t.Setenv("HTTP_PROXY", "http://proxy.example:8080")
	t.Setenv("HTTPS_PROXY", "http://proxy.example:8080")
	opts := newOptions()
	c := newCircuitBreakerClient(*opts, newHTTPClientMetrics(opts.collector, opts.metricsSubsystem))
	require.Nil(t, c.client.Transport.(*http.Transport).Proxy)
	require.NotNil(t, opts.client.Transport.(*http.Transport).Proxy, "caller transport must not be changed")
}

func TestSSRFLoopbackPolicy(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	t.Cleanup(srv.Close)
	for _, tc := range []struct {
		name    string
		opts    []Option
		blocked bool
	}{
		{"default", nil, true},
		{"allowed", []Option{WithSSRFAllowedCIDRs(netip.MustParsePrefix("127.0.0.0/8"))}, false},
		{"mapped allowed", []Option{WithSSRFAllowedCIDRs(netip.MustParsePrefix("::ffff:127.0.0.0/104"))}, false},
		{"disabled", []Option{WithoutSSRFProtection()}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := New(append(tc.opts, WithRetryMax(0), WithoutProxy())...)
			t.Cleanup(c.CloseIdleConnections)
			resp, err := c.Get(srv.URL)
			if tc.blocked {
				require.ErrorIs(t, err, ErrSSRFBlocked)
				return
			}
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
		})
	}
}
