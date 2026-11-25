// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client

import (
	"bytes"
	"context"
	"crypto/elliptic"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/jwt"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	"golang.org/x/oauth2"
)

func TestRedactErrorWrappedAndJoinedDescendants(t *testing.T) {
	t.Parallel()
	cause := compositeRetrieveError("original-response-secret")
	redacted := redactError(cause)
	require.Same(t, redacted, redactError(redacted))
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"wrapped", fmt.Errorf("outer wrapper: %w", redacted)},
		{"joined", errors.Join(context.Canceled, redacted, compositeRetrieveError("fresh-response-secret"))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := redactError(tc.err)
			require.Equal(t, redacted.Error(), got.Error())
			require.ErrorIs(t, got, tc.err)
			require.ErrorIs(t, got, cause)
			var retrieved *oauth2.RetrieveError
			require.ErrorAs(t, got, &retrieved)
			require.Same(t, cause, retrieved)
		})
	}
}

func TestRedactErrorWrappedDecodedResponse(t *testing.T) {
	t.Parallel()
	_, cause := strconv.ParseInt("decoded-response-secret", 10, 64)
	require.Error(t, cause)
	redacted := responseError(cause, http.StatusOK, 42)
	wrapped := fmt.Errorf("decoded-response-secret: %w", redacted)
	got := redactError(wrapped)
	require.Equal(t, redacted.Error(), got.Error())
	require.ErrorIs(t, got, wrapped)
	require.ErrorIs(t, got, cause)
	var decoded *strconv.NumError
	require.ErrorAs(t, got, &decoded)
}

func TestCompositeTransportFailureRedaction(t *testing.T) {
	t.Parallel()
	for _, flow := range []string{
		"client credentials", "early expiry", "client assertion", "refresh", "authorization code",
		"device authorization", "device token", "exchange", "jwt bearer", "revocation",
		"retry disabled", "retry exhausted", "unreplayable",
	} {
		t.Run(flow, func(t *testing.T) {
			t.Parallel()
			for _, kind := range []string{"retrieve", "redacted descendant", "decoded descendant", "connection refused"} {
				t.Run(kind, func(t *testing.T) {
					t.Parallel()
					const secret = "fresh-composite-response-secret"
					retrieved := compositeRetrieveError(secret)
					var cause error = errors.Join(context.DeadlineExceeded, retrieved)
					if kind == "redacted descendant" {
						cause = errors.Join(context.DeadlineExceeded, redactError(compositeRetrieveError("previous-response-secret")), retrieved)
					} else if kind == "decoded descendant" {
						_, decoded := strconv.ParseInt(secret, 10, 64)
						cause = errors.Join(context.DeadlineExceeded, fmt.Errorf("%s: %w", secret, responseError(decoded, http.StatusOK, len(secret))))
					} else if kind == "connection refused" {
						cause = syscall.ECONNREFUSED
					}
					var logs bytes.Buffer
					client := &http.Client{Transport: testhelpers.RoundTripFunc(func(req *http.Request) (*http.Response, error) {
						if req.Body != nil {
							_ = req.Body.Close()
						}
						return nil, cause
					})}
					err := runCompositeFailureFlow(t, flow, client, slog.New(slog.NewJSONHandler(&logs, nil)))
					require.ErrorIs(t, err, cause)
					require.NotContains(t, logs.String(), secret)
					require.NotContains(t, logs.String(), "previous-response-secret")
					require.NotContains(t, err.Error(), secret)
					require.NotContains(t, err.Error(), "previous-response-secret")
					if kind == "connection refused" {
						require.ErrorIs(t, err, syscall.ECONNREFUSED)
						if flow != "jwt bearer" {
							require.Contains(t, err.Error(), syscall.ECONNREFUSED.Error())
						}
						return
					}
					require.ErrorIs(t, err, context.DeadlineExceeded)
					if kind == "decoded descendant" {
						var decoded *strconv.NumError
						require.ErrorAs(t, err, &decoded)
					} else {
						require.ErrorIs(t, err, retrieved)
						var got *oauth2.RetrieveError
						require.ErrorAs(t, err, &got)
					}
					if flow != "revocation" && flow != "device authorization" &&
						flow != "retry disabled" && flow != "retry exhausted" && flow != "unreplayable" {
						require.NotEmpty(t, logs.String())
					}
				})
			}
		})
	}
}

func compositeRetrieveError(secret string) *oauth2.RetrieveError {
	return &oauth2.RetrieveError{
		Response:         &http.Response{StatusCode: http.StatusUnauthorized},
		Body:             []byte(secret),
		ErrorCode:        "invalid_client",
		ErrorDescription: secret,
		ErrorURI:         "https://idp.example/" + secret,
	}
}

func runCompositeFailureFlow(t testing.TB, flow string, client *http.Client, logger *slog.Logger) error {
	t.Helper()
	const endpointURL = "https://idp.example/token"
	endpoint := oauth2.Endpoint{TokenURL: endpointURL, DeviceAuthURL: endpointURL, AuthStyle: oauth2.AuthStyleInHeader}
	switch flow {
	case "client credentials", "early expiry", "client assertion":
		opts := []ClientCredentialsOption{
			WithClientCredentialsHTTPClient(client), WithClientCredentialsLogger(logger), WithClientCredentialsAuthStyle(oauth2.AuthStyleInHeader),
		}
		if flow == "early expiry" {
			opts = append(opts, WithClientCredentialsEarlyExpiry(time.Minute))
		} else if flow == "client assertion" {
			auth, err := ClientSecretJWT("client", "secret")
			require.NoError(t, err)
			opts = append(opts, WithClientCredentialsClientAuth(auth))
		}
		source, err := ClientCredentials(t.Context(), endpointURL, "client", "secret", opts...)
		require.NoError(t, err)
		_, err = source.Token()
		return err
	case "refresh":
		source, err := Refresh(t.Context(), endpointURL, "client", "secret", "refresh",
			WithRefreshHTTPClient(client), WithRefreshLogger(logger), WithRefreshAuthStyle(oauth2.AuthStyleInHeader))
		require.NoError(t, err)
		_, err = source.Token()
		return err
	case "authorization code":
		flow, err := NewAuthCode(endpoint, "client", "secret", "https://app.example/callback",
			WithAuthCodeHTTPClient(client), WithAuthCodeLogger(logger))
		require.NoError(t, err)
		_, _, err = flow.Exchange(t.Context(), "code")
		return err
	case "device authorization", "device token":
		device, err := NewDeviceFlow(endpoint, "client", "secret",
			WithDeviceFlowHTTPClient(client), WithDeviceFlowLogger(logger))
		require.NoError(t, err)
		if flow == "device authorization" {
			_, err = device.Authorize(t.Context())
		} else {
			_, _, err = device.Token(t.Context(), &oauth2.DeviceAuthResponse{DeviceCode: "code", Interval: 1})
		}
		return err
	case "exchange":
		exchanger, err := NewExchanger(endpointURL, "client", "secret",
			WithExchangerHTTPClient(client),
			WithExchangerLogger(logger),
			WithExchangerAuthStyle(oauth2.AuthStyleInHeader),
			WithExchangerRetryAttempts(1),
			WithExchangerRetryBaseDelay(time.Millisecond),
			WithExchangerRetryMaxDelay(time.Millisecond))
		require.NoError(t, err)
		_, err = exchanger.Exchange(t.Context(), ExchangeRequest{SubjectToken: "subject"})
		return err
	case "jwt bearer":
		key := jwt.SigningKey{KeyID: "key", Algorithm: jwt.AlgES256, Key: testhelpers.GenerateECDSAKey(t, elliptic.P256())}
		bearer, err := NewJWTBearer(endpointURL, "client", key,
			WithJWTBearerHTTPClient(client),
			WithJWTBearerLogger(logger),
			WithJWTBearerRetryAttempts(1),
			WithJWTBearerRetryBaseDelay(time.Millisecond),
			WithJWTBearerRetryMaxDelay(time.Millisecond))
		require.NoError(t, err)
		_, err = bearer.Token(t.Context())
		return err
	case "revocation":
		revoker, err := NewRevoker(endpointURL, "client", "secret",
			WithRevokerHTTPClient(client), WithRevokerAuthStyle(oauth2.AuthStyleInHeader))
		require.NoError(t, err)
		return revoker.Revoke(t.Context(), "token")
	default:
		attempts := 1
		if flow == "retry disabled" {
			attempts = 0
		}
		transport := RetryTransport(client.Transport, WithTransportAttempts(attempts), WithTransportBackoff(time.Millisecond, time.Millisecond))
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpointURL, strings.NewReader("request"))
		require.NoError(t, err)
		if flow == "unreplayable" {
			req.GetBody = nil
		}
		resp, err := transport.RoundTrip(req)
		if resp != nil {
			_ = resp.Body.Close()
		}
		return err
	}
}

func TestDoFormRequestFailureAfterResponseRedaction(t *testing.T) {
	t.Parallel()
	const secret = "redirect-response-secret"
	client := &http.Client{Transport: testhelpers.RoundTripFunc(func(req *http.Request) (*http.Response, error) {
		_ = req.Body.Close()
		return &http.Response{
			StatusCode: http.StatusTemporaryRedirect,
			Header:     http.Header{"Location": {"https://other.example/" + secret}},
			Body:       io.NopCloser(strings.NewReader(secret)),
			Request:    req,
		}, nil
	}), CheckRedirect: func(*http.Request, []*http.Request) error { return nil }}
	var logs bytes.Buffer
	err := runCompositeFailureFlow(t, "exchange", client, slog.New(slog.NewJSONHandler(&logs, nil)))
	require.ErrorIs(t, err, ErrRedirectNotAllowed)
	require.NotContains(t, err.Error(), secret)
	require.NotContains(t, logs.String(), secret)
}
