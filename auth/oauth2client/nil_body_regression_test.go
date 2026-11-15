// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client_test

import (
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/altessa-s/go-atlas/auth/oauth2client"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func TestSuccessfulNilResponseBody(t *testing.T) {
	t.Parallel()
	const endpointURL = "https://idp.example/token"
	endpoint := oauth2.Endpoint{TokenURL: endpointURL, DeviceAuthURL: endpointURL, AuthStyle: oauth2.AuthStyleInHeader}
	for _, tc := range []struct {
		name string
		run  func(*testing.T, ...oauth2client.Option) error
	}{
		{"client credentials", func(t *testing.T, opts ...oauth2client.Option) error {
			_, err := mustClientCredentials(t, endpointURL, "svc", clientSecret, opts...).Token()
			return err
		}},
		{"client credentials with early expiry", func(t *testing.T, opts ...oauth2client.Option) error {
			opts = append(opts, oauth2client.WithEarlyExpiry(time.Minute))
			_, err := mustClientCredentials(t, endpointURL, "svc", clientSecret, opts...).Token()
			return err
		}},
		{"client credentials with assertion", func(t *testing.T, opts ...oauth2client.Option) error {
			auth, err := oauth2client.ClientSecretJWT("svc", clientSecret)
			require.NoError(t, err)
			opts = append(opts, oauth2client.WithClientAuth(auth))
			_, err = mustClientCredentials(t, endpointURL, "svc", "", opts...).Token()
			return err
		}},
		{"refresh", func(t *testing.T, opts ...oauth2client.Option) error {
			_, err := mustRefresh(t, endpointURL, "svc", clientSecret, refreshToken, opts...).Token()
			return err
		}},
		{"authorization code", func(t *testing.T, opts ...oauth2client.Option) error {
			flow, err := oauth2client.NewAuthCode(endpoint, "svc", clientSecret, "https://app.example/cb", opts...)
			require.NoError(t, err)
			_, _, err = flow.Exchange(t.Context(), "code")
			return err
		}},
		{"device authorization", func(t *testing.T, opts ...oauth2client.Option) error {
			_, err := mustDeviceFlow(t, endpoint, "svc", clientSecret, opts...).Authorize(t.Context())
			return err
		}},
		{"device token", func(t *testing.T, opts ...oauth2client.Option) error {
			flow := mustDeviceFlow(t, endpoint, "svc", clientSecret, opts...)
			_, _, err := flow.Token(t.Context(), &oauth2.DeviceAuthResponse{DeviceCode: "device-code", Interval: 1})
			return err
		}},
		{"token exchange", func(t *testing.T, opts ...oauth2client.Option) error {
			_, err := mustExchanger(t, endpointURL, "svc", clientSecret, opts...).
				Exchange(t.Context(), oauth2client.ExchangeRequest{SubjectToken: bearerToken})
			return err
		}},
		{"jwt bearer", func(t *testing.T, opts ...oauth2client.Option) error {
			key, _ := rsaKey(t, "key")
			flow, err := oauth2client.NewJWTBearer(endpointURL, "svc", key, opts...)
			require.NoError(t, err)
			_, err = flow.Token(t.Context())
			return err
		}},
		{"revocation", func(t *testing.T, opts ...oauth2client.Option) error {
			return mustRevoker(t, endpointURL, "svc", clientSecret, opts...).Revoke(t.Context(), refreshToken)
		}},
		{"discovery client credentials", func(t *testing.T, opts ...oauth2client.Option) error {
			src, err := oauth2client.ClientCredentialsFromDiscovery(t.Context(), discoveredTokenURL(endpointURL), "svc", clientSecret, opts...)
			require.NoError(t, err)
			_, err = src.Token()
			return err
		}},
		{"discovery exchange", func(t *testing.T, opts ...oauth2client.Option) error {
			flow, err := oauth2client.NewExchangerFromDiscovery(discoveredTokenURL(endpointURL), "svc", clientSecret, opts...)
			require.NoError(t, err)
			_, err = flow.Exchange(t.Context(), oauth2client.ExchangeRequest{SubjectToken: bearerToken})
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var results []error
			for _, body := range []io.ReadCloser{http.NoBody, nil} {
				client := &http.Client{Transport: testhelpers.RoundTripFunc(func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusNoContent, ContentLength: 0, Body: body, Request: req,
						Header: http.Header{"Content-Type": []string{"application/json"}},
					}, nil
				})}
				require.NotPanics(t, func() {
					results = append(results, tc.run(t, oauth2client.WithHTTPClient(client),
						oauth2client.WithAuthStyle(oauth2.AuthStyleInHeader)))
				})
			}
			if tc.name == "revocation" {
				require.NoError(t, results[0])
				require.NoError(t, results[1])
				return
			}
			require.Error(t, results[0])
			require.Error(t, results[1])
			require.Equal(t, results[0].Error(), results[1].Error())
			require.Contains(t, results[1].Error(), "status 204")
			require.Contains(t, results[1].Error(), "response body of 0 bytes")
			require.NotContains(t, results[1].Error(), clientSecret)
		})
	}
}
