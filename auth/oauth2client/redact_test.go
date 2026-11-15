// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/altessa-s/go-atlas/auth/oauth2client"
)

const (
	clientSecret = "s3cr3t"
	refreshToken = "rt-123"
	bearerToken  = "eyJhbGciOiJIUzI1NiJ9.e30.c2lnbmF0dXJl"
)

// An IdP that echoes credentials in its error response must not get them into
// a log record or into the text of the returned error, whatever the flow.
func TestErrorResponseSecretsStayOutOfLogsAndErrors(t *testing.T) {
	t.Parallel()

	responses := []redactionResponse{
		{
			name:        "RFC 6749 JSON error echoing credentials",
			status:      http.StatusBadRequest,
			contentType: "application/json",
			body: `{"error":"invalid_client","error_description":"bad client_secret=` + clientSecret +
				`","client_secret":"` + clientSecret + `","refresh_token":"` + refreshToken + `"}`,
			wantInError: `error "invalid_client" (description: 24 bytes withheld)`,
		},
		{
			name:        "percent-encoded credential in the description",
			status:      http.StatusBadRequest,
			contentType: "application/json",
			body:        `{"error":"invalid_client","error_description":"bad client_secret%3D` + clientSecret + ` Bearer%20` + bearerToken + `"}`,
			wantInError: `error "invalid_client" (description: `,
		},
		{
			name:        "single-quoted credential in the description",
			status:      http.StatusBadRequest,
			contentType: "application/json",
			body:        `{"error":"invalid_client","error_description":"got 'client_secret': '` + clientSecret + `'"}`,
			wantInError: `error "invalid_client" (description: `,
		},
		{
			name:        "underscore-prefixed parameter names in the description",
			status:      http.StatusBadRequest,
			contentType: "application/json",
			body:        `{"error":"invalid_client","error_description":"api_secret=` + clientSecret + ` oauth_token=` + refreshToken + `"}`,
			wantInError: `error "invalid_client" (description: `,
		},
		{
			name:        "plain description is withheld",
			status:      http.StatusBadRequest,
			contentType: "application/json",
			body:        `{"error":"invalid_scope","error_description":"The requested scope is not allowed for this client."}`,
			wantInError: `error "invalid_scope" (description: 51 bytes withheld)`,
		},
		{
			name:        "non-JSON body with a bearer token",
			status:      http.StatusServiceUnavailable,
			contentType: "text/plain",
			body:        "upstream rejected Authorization: Bearer " + bearerToken + " refresh_token=" + refreshToken,
		},
	}

	runRedactionResponses(t, responses)
}

type redactionResponse struct {
	name        string
	status      int
	contentType string
	body        string
	wantInError string
}

func runRedactionResponses(t *testing.T, responses []redactionResponse) {
	t.Helper()

	fast := func(logger *slog.Logger, extra ...oauth2client.Option) []oauth2client.Option {
		return append([]oauth2client.Option{
			oauth2client.WithLogger(logger),
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

	for _, resp := range responses {
		for name, run := range flows {
			t.Run(resp.name+"/"+name, func(t *testing.T) {
				t.Parallel()
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", resp.contentType)
					w.WriteHeader(resp.status)
					_, _ = w.Write([]byte(resp.body))
				}))
				t.Cleanup(srv.Close)
				var logs bytes.Buffer
				logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

				err := run(t, srv.URL, logger)

				require.Error(t, err)
				if re, ok := errors.AsType[*oauth2.RetrieveError](err); ok {
					require.Equal(t, resp.body, string(re.Body))
					var fields struct {
						Description string `json:"error_description"`
						URI         string `json:"error_uri"`
					}
					if json.Unmarshal([]byte(resp.body), &fields) == nil {
						require.Equal(t, fields.Description, re.ErrorDescription)
						require.Equal(t, fields.URI, re.ErrorURI)
					}
				}
				for _, secret := range []string{clientSecret, refreshToken, bearerToken, "The requested scope is not allowed for this client."} {
					require.NotContains(t, err.Error(), secret)
					require.NotContains(t, fmt.Sprintf("%+v", err), secret)
					require.NotContains(t, logs.String(), secret)
				}
				require.Contains(t, err.Error(), fmt.Sprintf("status %d", resp.status))
				if resp.wantInError != "" {
					require.Contains(t, err.Error(), resp.wantInError)
				} else {
					require.Contains(t, err.Error(), fmt.Sprintf("response body of %d bytes", len(resp.body)))
				}
			})
		}
	}
}

// The x/oauth2 error stays reachable behind the redacted text, so callers can
// still branch on the status and the RFC 6749 error code.
func TestRedactedErrorKeepsRetrieveError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_client","error_description":"password: hunter2"}`))
	}))
	t.Cleanup(srv.Close)

	_, err := mustClientCredentials(t, srv.URL, "svc", "pw",
		oauth2client.WithAuthStyle(oauth2.AuthStyleInHeader)).Token()

	re, ok := errors.AsType[*oauth2.RetrieveError](err)
	require.True(t, ok)
	require.Equal(t, http.StatusUnauthorized, re.Response.StatusCode)
	require.Equal(t, "invalid_client", re.ErrorCode)
	require.ErrorIs(t, err, re)
	require.NotContains(t, err.Error(), "hunter2")
}
