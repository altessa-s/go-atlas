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

	"github.com/altessa-s/go-atlas/auth/oauth2client"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/stretchr/testify/require"
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
	fast := func(logger *slog.Logger, extra ...oauth2client.Option) []oauth2client.Option {
		return append([]oauth2client.Option{
			oauth2client.WithLogger(logger),
			oauth2client.WithHTTPClient(client),
			oauth2client.WithAuthStyle(oauth2.AuthStyleInHeader),
			oauth2client.WithRetryAttempts(1),
			oauth2client.WithRetryBaseDelay(time.Millisecond),
			oauth2client.WithRetryMaxDelay(time.Millisecond),
		}, extra...)
	}
	endpoint := func(url string) oauth2.Endpoint {
		return oauth2.Endpoint{TokenURL: url, DeviceAuthURL: url, AuthStyle: oauth2.AuthStyleInHeader}
	}
	flows := map[string]func(t *testing.T, url string, logger *slog.Logger) error{
		"client credentials": func(t *testing.T, url string, logger *slog.Logger) error {
			_, err := mustClientCredentials(t, url, "svc", clientSecret, fast(logger)...).Token()
			return err
		},
		"client credentials with early expiry": func(t *testing.T, url string, logger *slog.Logger) error {
			_, err := mustClientCredentials(t, url, "svc", clientSecret,
				fast(logger, oauth2client.WithEarlyExpiry(time.Minute))...).Token()
			return err
		},
		"client credentials with a client assertion": func(t *testing.T, url string, logger *slog.Logger) error {
			auth, err := oauth2client.ClientSecretJWT("svc", clientSecret)
			require.NoError(t, err)
			_, err = mustClientCredentials(t, url, "svc", "",
				fast(logger, oauth2client.WithClientAuth(auth))...).Token()
			return err
		},
		"refresh": func(t *testing.T, url string, logger *slog.Logger) error {
			_, err := mustRefresh(t, url, "svc", clientSecret, refreshToken, fast(logger)...).Token()
			return err
		},
		"authorization code": func(t *testing.T, url string, logger *slog.Logger) error {
			ac, err := oauth2client.NewAuthCode(endpoint(url), "svc", clientSecret, "https://app.example/cb", fast(logger)...)
			require.NoError(t, err)
			_, _, err = ac.Exchange(t.Context(), "code")
			return err
		},
		"device authorization": func(t *testing.T, url string, logger *slog.Logger) error {
			_, err := mustDeviceFlow(t, endpoint(url), "svc", clientSecret, fast(logger)...).Authorize(t.Context())
			return err
		},
		"device token": func(t *testing.T, url string, logger *slog.Logger) error {
			df := mustDeviceFlow(t, endpoint(url), "svc", clientSecret, fast(logger)...)
			_, _, err := df.Token(t.Context(), &oauth2.DeviceAuthResponse{DeviceCode: "dev-code", Interval: 1})
			return err
		},
		"token exchange": func(t *testing.T, url string, logger *slog.Logger) error {
			_, err := mustExchanger(t, url, "svc", clientSecret, fast(logger)...).
				Exchange(t.Context(), oauth2client.ExchangeRequest{SubjectToken: bearerToken})
			return err
		},
		"jwt bearer": func(t *testing.T, url string, logger *slog.Logger) error {
			key, _ := rsaKey(t, "k")
			b, err := oauth2client.NewJWTBearer(url, "svc", key, fast(logger)...)
			require.NoError(t, err)
			_, err = b.Token(t.Context())
			return err
		},
		"revocation": func(t *testing.T, url string, logger *slog.Logger) error {
			return mustRevoker(t, url, "svc", clientSecret, fast(logger)...).Revoke(t.Context(), refreshToken)
		},
	}

	flows["discovery client credentials"] = func(t *testing.T, url string, logger *slog.Logger) error {
		src, err := oauth2client.ClientCredentialsFromDiscovery(t.Context(), discoveredTokenURL(url), "svc", clientSecret, fast(logger)...)
		require.NoError(t, err)
		_, err = src.Token()
		return err
	}
	flows["discovery exchange"] = func(t *testing.T, url string, logger *slog.Logger) error {
		ex, err := oauth2client.NewExchangerFromDiscovery(discoveredTokenURL(url), "svc", clientSecret, fast(logger)...)
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
				require.NotNil(t, errors.Unwrap(err))
			}
			require.NotContains(t, logs.String(), clientSecret)
		})
	}
}
