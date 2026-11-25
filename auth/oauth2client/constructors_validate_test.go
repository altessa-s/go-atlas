// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/oauth2client"

	"golang.org/x/oauth2"
)

// Every constructor checks its endpoint URLs up front with the same rule: an
// https URL and a loopback http URL are accepted, any other http URL only
// with the grant’s plain-HTTP opt-in, and a malformed or missing URL never.
func TestConstructorsRejectInsecureEndpointsUpFront(t *testing.T) {
	t.Parallel()

	const good = "https://idp.example/endpoint"
	key, _ := rsaKey(t, "k")
	// Each constructor is called with one endpoint under test; its other
	// endpoints, if any, are valid.
	constructors := map[string]func(t *testing.T, url string, allowHTTP bool) error{
		"ClientCredentials": func(t *testing.T, url string, allowHTTP bool) error {
			_, err := oauth2client.ClientCredentials(t.Context(), url, "svc", "s3cr3t", allowInsecureHTTP(allowHTTP,
				oauth2client.WithClientCredentialsInsecureHTTP)...)
			return err
		},
		"ClientCredentialsFromDiscovery": func(t *testing.T, url string, allowHTTP bool) error {
			_, err := oauth2client.ClientCredentialsFromDiscovery(t.Context(), staticEndpoint(url), "svc", "s3cr3t", allowInsecureHTTP(allowHTTP,
				oauth2client.WithClientCredentialsInsecureHTTP)...)
			return err
		},
		"Refresh": func(t *testing.T, url string, allowHTTP bool) error {
			_, err := oauth2client.Refresh(t.Context(), url, "svc", "s3cr3t", "refresh-token", allowInsecureHTTP(allowHTTP,
				oauth2client.WithRefreshInsecureHTTP)...)
			return err
		},
		"NewExchanger": func(_ *testing.T, url string, allowHTTP bool) error {
			_, err := oauth2client.NewExchanger(url, "svc", "s3cr3t", allowInsecureHTTP(allowHTTP, oauth2client.WithExchangerInsecureHTTP)...)
			return err
		},
		"NewExchangerFromDiscovery": func(_ *testing.T, url string, allowHTTP bool) error {
			_, err := oauth2client.NewExchangerFromDiscovery(staticEndpoint(url), "svc", "s3cr3t", allowInsecureHTTP(allowHTTP,
				oauth2client.WithExchangerInsecureHTTP)...)
			return err
		},
		"NewRevoker": func(_ *testing.T, url string, allowHTTP bool) error {
			_, err := oauth2client.NewRevoker(url, "svc", "s3cr3t", allowInsecureHTTP(allowHTTP, oauth2client.WithRevokerInsecureHTTP)...)
			return err
		},
		"NewDeviceFlow/device authorization URL": func(_ *testing.T, url string, allowHTTP bool) error {
			_, err := oauth2client.NewDeviceFlow(oauth2.Endpoint{DeviceAuthURL: url, TokenURL: good}, "svc", "", allowInsecureHTTP(allowHTTP,
				oauth2client.WithDeviceFlowInsecureHTTP)...)
			return err
		},
		"NewDeviceFlow/token URL": func(_ *testing.T, url string, allowHTTP bool) error {
			_, err := oauth2client.NewDeviceFlow(oauth2.Endpoint{DeviceAuthURL: good, TokenURL: url}, "svc", "", allowInsecureHTTP(allowHTTP,
				oauth2client.WithDeviceFlowInsecureHTTP)...)
			return err
		},
		"NewAuthCode/authorization URL": func(_ *testing.T, url string, allowHTTP bool) error {
			_, err := oauth2client.NewAuthCode(oauth2.Endpoint{AuthURL: url, TokenURL: good}, "svc", "s3cr3t", "https://app.example/cb", allowInsecureHTTP(allowHTTP,
				oauth2client.WithAuthCodeInsecureHTTP)...)
			return err
		},
		"NewAuthCode/token URL": func(_ *testing.T, url string, allowHTTP bool) error {
			_, err := oauth2client.NewAuthCode(oauth2.Endpoint{AuthURL: good, TokenURL: url}, "svc", "s3cr3t", "https://app.example/cb", allowInsecureHTTP(allowHTTP,
				oauth2client.WithAuthCodeInsecureHTTP)...)
			return err
		},
		"NewJWTBearer": func(_ *testing.T, url string, allowHTTP bool) error {
			_, err := oauth2client.NewJWTBearer(url, "svc", key, allowInsecureHTTP(allowHTTP, oauth2client.WithJWTBearerInsecureHTTP)...)
			return err
		},
	}
	urls := []struct {
		name      string
		url       string
		allowHTTP bool
		wantErr   bool
	}{
		{name: "https", url: good},
		{name: "loopback http", url: "http://127.0.0.1:8080/endpoint"},
		{name: "localhost http", url: "http://localhost:8080/endpoint"},
		{name: "http explicitly allowed", url: "http://idp.example/endpoint", allowHTTP: true},
		{name: "http", url: "http://idp.example/endpoint", wantErr: true},
		{name: "no host", url: "https:///endpoint", wantErr: true},
		{name: "not a URL", url: "://idp.example", wantErr: true},
		{name: "other scheme", url: "ftp://idp.example/endpoint", wantErr: true},
	}

	for name, construct := range constructors {
		for _, tc := range urls {
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				err := construct(t, tc.url, tc.allowHTTP)
				if tc.wantErr {
					require.ErrorIs(t, err, oauth2client.ErrInsecureEndpoint)
					return
				}
				require.NoError(t, err)
			})
		}
	}
}
