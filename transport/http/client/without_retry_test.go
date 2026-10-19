// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	httpclient "github.com/altessa-s/go-atlas/transport/http/client"
)

// loopback lets the SSRF-protected client reach httptest servers.
var loopback = httpclient.WithSSRFAllowedCIDRs(netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("::1/128"))

func TestWithoutRetry_ReturnsEveryStatusOnce(t *testing.T) {
	t.Parallel()

	for _, code := range []int{http.StatusOK, http.StatusUnauthorized, http.StatusTooManyRequests,
		http.StatusInternalServerError, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			t.Parallel()

			var hits atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				hits.Add(1)
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(code)
				_, _ = io.WriteString(w, "provider body")
			}))
			t.Cleanup(srv.Close)

			policyCalled := false
			c := httpclient.New(loopback, httpclient.WithoutRetry(),
				// WithoutRetry overrides both.
				httpclient.WithRetryMax(5),
				httpclient.WithRetryPolicyHandler(func(context.Context, *http.Response, error) (bool, error) {
					policyCalled = true
					return true, nil
				}))

			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
			require.NoError(t, err)
			resp, err := c.Do(req)
			require.NoError(t, err, "a non-2xx reply is not an error")
			t.Cleanup(func() { _ = resp.Body.Close() })

			require.Equal(t, code, resp.StatusCode)
			require.Equal(t, "1", resp.Header.Get("Retry-After"))
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, "provider body", string(body), "the body reaches the caller unread")
			require.Equal(t, int32(1), hits.Load(), "exactly one attempt")
			require.False(t, policyCalled)
		})
	}
}

func TestWithoutRetry_TransportErrorOnce(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		hj, ok := w.(http.Hijacker)
		require.True(t, ok)
		conn, _, err := hj.Hijack()
		require.NoError(t, err)
		_ = conn.Close() // drop the connection without a reply
	}))
	t.Cleanup(srv.Close)

	c := httpclient.New(loopback, httpclient.WithoutRetry())
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL, nil)
	require.NoError(t, err)
	resp, err := c.Do(req)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.Error(t, err)
	require.Nil(t, httpclient.IsUnexpectedStatusError(err))
	require.Equal(t, int32(1), hits.Load(), "a transport error is not retried")
}
