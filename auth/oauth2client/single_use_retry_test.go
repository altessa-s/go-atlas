// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client_test

import (
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/oauth2client"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	"golang.org/x/oauth2"
)

func TestRefreshGrantIsNotRetriedAfterBadGateway(t *testing.T) {
	t.Parallel()
	var posts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.PostFormValue("grant_type") == "refresh_token" {
			posts.Add(1)
		}
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(server.Close)
	client := &http.Client{Transport: oauth2client.RetryTransport(server.Client().Transport,
		oauth2client.WithTransportBackoff(time.Millisecond, time.Millisecond))}
	src, err := oauth2client.Refresh(t.Context(), server.URL, "client", "secret", "single-use",
		oauth2client.WithRefreshHTTPClient(client), oauth2client.WithRefreshAuthStyle(oauth2.AuthStyleInHeader))
	require.NoError(t, err)
	_, err = src.Token()
	require.Error(t, err)
	require.EqualValues(t, 1, posts.Load())
}

func TestSingleUseGrantRetryPolicy(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		grant        string
		result       func(*http.Request) (*http.Response, error)
		insecure     bool
		wantAttempts int
	}{
		{"dial failure", "refresh_token", func(*http.Request) (*http.Response, error) {
			return nil, &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}
		}, false, 2},
		{"TLS handshake failure", "authorization_code", func(req *http.Request) (*http.Response, error) {
			err := errors.New("TLS handshake failed")
			httptrace.ContextClientTrace(req.Context()).TLSHandshakeDone(tls.ConnectionState{}, err)
			return nil, err
		}, false, 2},
		{"ambiguous connection reset", "authorization_code", func(*http.Request) (*http.Response, error) {
			return nil, syscall.ECONNRESET
		}, false, 1},
		{"device rate limit", "urn:ietf:params:oauth:grant-type:device_code", func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusTooManyRequests, Body: http.NoBody}, nil
		}, false, 1},
		{"explicit replay", "refresh_token", func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusBadGateway, Body: http.NoBody}, nil
		}, true, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				attempts := 0
				base := testhelpers.RoundTripFunc(func(req *http.Request) (*http.Response, error) {
					attempts++
					defer req.Body.Close()
					_, err := io.Copy(io.Discard, req.Body)
					require.NoError(t, err)
					return tc.result(req)
				})
				opts := []oauth2client.RetryTransportOption{
					oauth2client.WithTransportAttempts(1),
					oauth2client.WithTransportBackoff(time.Millisecond, time.Millisecond),
				}
				if tc.insecure {
					opts = append(opts, oauth2client.WithInsecureRetrySingleUseGrants())
				}
				req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://idp.example/token",
					strings.NewReader("grant_type="+tc.grant))
				require.NoError(t, err)
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				resp, err := oauth2client.RetryTransport(base, opts...).RoundTrip(req)
				if resp != nil {
					require.NoError(t, resp.Body.Close())
				} else {
					require.Error(t, err)
				}
				require.Equal(t, tc.wantAttempts, attempts)
			})
		})
	}
}
