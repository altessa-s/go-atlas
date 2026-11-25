// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-net/oauth2/jwtbearer"
	"github.com/altessa-s/go-atlas/auth/oauth2client"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	"golang.org/x/oauth2"
)

type discoveredTokenURL string

func (u discoveredTokenURL) TokenEndpoint() string { return string(u) }

func TestMalformedSuccessResponseRedaction(t *testing.T) {
	t.Parallel()
	const body = `{"access_token":"x","expires_in":"s3cr3t"}`
	client := &http.Client{Transport: testhelpers.RoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
	})}
	endpoint := func(url string) oauth2.Endpoint {
		return oauth2.Endpoint{TokenURL: url, DeviceAuthURL: url, AuthStyle: oauth2.AuthStyleInHeader}
	}
	flows := map[string]func(t *testing.T, url string, logger *slog.Logger) error{
		"client credentials": func(t *testing.T, url string, logger *slog.Logger) error {
			_, err := mustClientCredentials(t, url, "svc", clientSecret,
				oauth2client.WithClientCredentialsHTTPClient(client),
				oauth2client.WithClientCredentialsLogger(logger),
				oauth2client.WithClientCredentialsAuthStyle(oauth2.AuthStyleInHeader)).Token()
			return err
		},
		"client credentials with early expiry": func(t *testing.T, url string, logger *slog.Logger) error {
			_, err := mustClientCredentials(t, url, "svc", clientSecret,
				oauth2client.WithClientCredentialsHTTPClient(client),
				oauth2client.WithClientCredentialsLogger(logger),
				oauth2client.WithClientCredentialsAuthStyle(oauth2.AuthStyleInHeader),
				oauth2client.WithClientCredentialsEarlyExpiry(time.Minute)).Token()
			return err
		},
		"client credentials with a client assertion": func(t *testing.T, url string, logger *slog.Logger) error {
			auth, err := oauth2client.ClientSecretJWT("svc", clientSecret)
			require.NoError(t, err)
			_, err = mustClientCredentials(t, url, "svc", "",
				oauth2client.WithClientCredentialsHTTPClient(client),
				oauth2client.WithClientCredentialsLogger(logger),
				oauth2client.WithClientCredentialsAuthStyle(oauth2.AuthStyleInHeader),
				oauth2client.WithClientCredentialsClientAuth(auth)).Token()
			return err
		},
		"refresh": func(t *testing.T, url string, logger *slog.Logger) error {
			_, err := mustRefresh(t, url, "svc", clientSecret, refreshToken,
				oauth2client.WithRefreshHTTPClient(client),
				oauth2client.WithRefreshLogger(logger),
				oauth2client.WithRefreshAuthStyle(oauth2.AuthStyleInHeader)).Token()
			return err
		},
		"authorization code": func(t *testing.T, url string, logger *slog.Logger) error {
			ac, err := oauth2client.NewAuthCode(endpoint(url), "svc", clientSecret, "https://app.example/cb",
				oauth2client.WithAuthCodeHTTPClient(client), oauth2client.WithAuthCodeLogger(logger))
			require.NoError(t, err)
			_, _, err = ac.Exchange(t.Context(), "code")
			return err
		},
		"device authorization": func(t *testing.T, url string, logger *slog.Logger) error {
			_, err := mustDeviceFlow(t, endpoint(url), "svc", clientSecret,
				oauth2client.WithDeviceFlowHTTPClient(client), oauth2client.WithDeviceFlowLogger(logger)).Authorize(t.Context())
			return err
		},
		"device token": func(t *testing.T, url string, logger *slog.Logger) error {
			df := mustDeviceFlow(t, endpoint(url), "svc", clientSecret,
				oauth2client.WithDeviceFlowHTTPClient(client), oauth2client.WithDeviceFlowLogger(logger))
			_, _, err := df.Token(t.Context(), &oauth2.DeviceAuthResponse{DeviceCode: "dev-code", Interval: 1})
			return err
		},
		"token exchange": func(t *testing.T, url string, logger *slog.Logger) error {
			_, err := mustExchanger(t, url, "svc", clientSecret,
				oauth2client.WithExchangerHTTPClient(client),
				oauth2client.WithExchangerLogger(logger),
				oauth2client.WithExchangerAuthStyle(oauth2.AuthStyleInHeader),
				oauth2client.WithExchangerRetryAttempts(1),
				oauth2client.WithExchangerRetryBaseDelay(time.Millisecond),
				oauth2client.WithExchangerRetryMaxDelay(time.Millisecond)).
				Exchange(t.Context(), oauth2client.ExchangeRequest{SubjectToken: bearerToken})
			return err
		},
		"jwt bearer": func(t *testing.T, url string, logger *slog.Logger) error {
			key, _ := rsaKey(t, "k")
			b, err := oauth2client.NewJWTBearer(url, "svc", key,
				oauth2client.WithJWTBearerHTTPClient(client),
				oauth2client.WithJWTBearerLogger(logger),
				oauth2client.WithJWTBearerRetryAttempts(1),
				oauth2client.WithJWTBearerRetryBaseDelay(time.Millisecond),
				oauth2client.WithJWTBearerRetryMaxDelay(time.Millisecond))
			require.NoError(t, err)
			_, err = b.Token(t.Context())
			return err
		},
		"revocation": func(t *testing.T, url string, logger *slog.Logger) error {
			return mustRevoker(t, url, "svc", clientSecret,
				oauth2client.WithRevokerHTTPClient(client), oauth2client.WithRevokerAuthStyle(oauth2.AuthStyleInHeader)).Revoke(t.Context(), refreshToken)
		},
	}

	flows["discovery client credentials"] = func(t *testing.T, url string, logger *slog.Logger) error {
		src, err := oauth2client.ClientCredentialsFromDiscovery(t.Context(), discoveredTokenURL(url), "svc", clientSecret,
			oauth2client.WithClientCredentialsHTTPClient(client),
			oauth2client.WithClientCredentialsLogger(logger),
			oauth2client.WithClientCredentialsAuthStyle(oauth2.AuthStyleInHeader))
		require.NoError(t, err)
		_, err = src.Token()
		return err
	}
	flows["discovery exchange"] = func(t *testing.T, url string, logger *slog.Logger) error {
		ex, err := oauth2client.NewExchangerFromDiscovery(discoveredTokenURL(url), "svc", clientSecret,
			oauth2client.WithExchangerHTTPClient(client),
			oauth2client.WithExchangerLogger(logger),
			oauth2client.WithExchangerAuthStyle(oauth2.AuthStyleInHeader),
			oauth2client.WithExchangerRetryAttempts(1),
			oauth2client.WithExchangerRetryBaseDelay(time.Millisecond),
			oauth2client.WithExchangerRetryMaxDelay(time.Millisecond))
		require.NoError(t, err)
		_, err = ex.Exchange(t.Context(), oauth2client.ExchangeRequest{SubjectToken: bearerToken})
		return err
	}
	for name, run := range flows {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
			err := run(t, "https://idp.example/token", logger)
			if name == "revocation" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.NotContains(t, err.Error(), clientSecret)
				require.Contains(t, err.Error(), "status 200")
				require.Contains(t, err.Error(), fmt.Sprintf("response body of %d bytes", len(body)))
				if name == "jwt bearer" {
					require.ErrorIs(t, err, jwtbearer.ErrResponse)
				} else {
					require.NotNil(t, errors.Unwrap(err))
				}
			}
			require.NotContains(t, logs.String(), clientSecret)
		})
	}
}
