// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/oauth2client"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	"golang.org/x/oauth2"
)

func TestAuthStyleRetryRetainsFinalTransportError(t *testing.T) {
	t.Parallel()
	for _, grant := range []string{"client credentials", "refresh", "authorization code"} {
		t.Run(grant, func(t *testing.T) {
			t.Parallel()
			var requests atomic.Int32
			client := &http.Client{Transport: testhelpers.RoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if requests.Add(1) == 1 {
					return &http.Response{
						StatusCode: http.StatusUnauthorized,
						Header:     http.Header{"Content-Type": {"application/json"}},
						Body:       io.NopCloser(strings.NewReader(`{"error":"invalid_client"}`)),
						Request:    req,
					}, nil
				}
				return nil, syscall.ECONNREFUSED
			})}
			const endpoint = "https://idp.example/token"
			var err error
			switch grant {
			case "client credentials":
				src, buildErr := oauth2client.ClientCredentials(t.Context(), endpoint, "client", "secret", oauth2client.WithClientCredentialsHTTPClient(client))
				require.NoError(t, buildErr)
				_, err = src.Token()
			case "refresh":
				src, buildErr := oauth2client.Refresh(t.Context(), endpoint, "client", "secret", "refresh", oauth2client.WithRefreshHTTPClient(client))
				require.NoError(t, buildErr)
				_, err = src.Token()
			case "authorization code":
				flow, buildErr := oauth2client.NewAuthCode(oauth2.Endpoint{TokenURL: endpoint}, "client", "secret", "https://app.example/callback",
					oauth2client.WithAuthCodeHTTPClient(client))
				require.NoError(t, buildErr)
				_, _, err = flow.Exchange(t.Context(), "code")
			}
			require.Equal(t, int32(2), requests.Load())
			require.ErrorIs(t, err, syscall.ECONNREFUSED)
			require.Contains(t, err.Error(), syscall.ECONNREFUSED.Error())
			require.NotContains(t, err.Error(), "status 401")
		})
	}
}

type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *cancelOnClose) Close() error {
	err := b.ReadCloser.Close()
	b.cancel()
	return err
}

func TestDevicePollingRetainsCancellation(t *testing.T) {
	t.Parallel()
	for _, duringRequest := range []bool{false, true} {
		name := "between polls"
		if duringRequest {
			name = "during next request"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var requests atomic.Int32
			client := &http.Client{Transport: testhelpers.RoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if requests.Add(1) > 1 {
					cancel()
					return nil, req.Context().Err()
				}
				var body io.ReadCloser = io.NopCloser(strings.NewReader(`{"error":"authorization_pending"}`))
				if !duringRequest {
					body = &cancelOnClose{ReadCloser: body, cancel: cancel}
				}
				return &http.Response{
					StatusCode: http.StatusBadRequest,
					Header:     http.Header{"Content-Type": {"application/json"}},
					Body:       body,
					Request:    req,
				}, nil
			})}
			const endpoint = "https://idp.example/token"
			flow, err := oauth2client.NewDeviceFlow(oauth2.Endpoint{
				TokenURL: endpoint, DeviceAuthURL: endpoint, AuthStyle: oauth2.AuthStyleInHeader,
			}, "client", "secret", oauth2client.WithDeviceFlowHTTPClient(client))
			require.NoError(t, err)
			_, _, err = flow.Token(ctx, &oauth2.DeviceAuthResponse{DeviceCode: "device", Interval: 1})
			require.ErrorIs(t, err, context.Canceled)
			require.ErrorIs(t, err, oauth2client.ErrDeviceAuth)
			require.Contains(t, err.Error(), context.Canceled.Error())
			require.NotContains(t, err.Error(), "status 400")
			wantRequests := int32(1)
			if duringRequest {
				wantRequests = 2
			}
			require.Equal(t, wantRequests, requests.Load())
		})
	}
}
