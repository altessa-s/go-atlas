// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client_test

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/altessa-s/go-atlas/auth/oauth2client"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

const tokenJSON = `{"access_token":"at","token_type":"Bearer","expires_in":3600}`

// redirectingIdP answers every request with a 307 to a second server and
// reports how many requests that second server received.
func redirectingIdP(t *testing.T) (tokenURL string, redirected *atomic.Int32) {
	t.Helper()
	redirected = &atomic.Int32{}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirected.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(tokenJSON))
	}))
	t.Cleanup(target.Close)
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(idp.Close)
	return idp.URL, redirected
}

// The built-in client must not follow a 307/308: that would re-POST the form
// with the client secret to the redirect target.
func TestDefaultClientDoesNotFollowRedirects(t *testing.T) {
	t.Parallel()

	t.Run("x/oauth2 path", func(t *testing.T) {
		t.Parallel()
		tokenURL, redirected := redirectingIdP(t)
		_, err := mustClientCredentials(t, tokenURL, "svc", "s3cr3t").Token()
		require.Error(t, err)
		require.Zero(t, redirected.Load(), "the secret must not be re-posted to the redirect target")
	})

	t.Run("hand-rolled path", func(t *testing.T) {
		t.Parallel()
		tokenURL, redirected := redirectingIdP(t)
		_, err := mustExchanger(t, tokenURL, "svc", "s3cr3t").
			Exchange(t.Context(), oauth2client.ExchangeRequest{SubjectToken: "subject"})
		require.ErrorIs(t, err, oauth2client.ErrTokenExchange)
		require.Zero(t, redirected.Load())
	})
}

// recordingClient counts the requests that reach the transport and answers
// them with a token.
func recordingClient() (*http.Client, *atomic.Int32) {
	var sent atomic.Int32
	return &http.Client{Transport: testhelpers.RoundTripFunc(func(*http.Request) (*http.Response, error) {
		sent.Add(1)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(tokenJSON)),
		}, nil
	})}, &sent
}

func TestInsecureEndpointIsRejected(t *testing.T) {
	t.Parallel()
	const insecureURL = "http://idp.example/token"

	flows := map[string]func(t *testing.T, opts ...oauth2client.Option) error{
		"client credentials": func(t *testing.T, opts ...oauth2client.Option) error {
			src, err := oauth2client.ClientCredentials(t.Context(), insecureURL, "svc", "s3cr3t", opts...)
			if err != nil {
				return err
			}
			_, err = src.Token()
			return err
		},
		"refresh": func(t *testing.T, opts ...oauth2client.Option) error {
			src, err := oauth2client.Refresh(t.Context(), insecureURL, "svc", "s3cr3t", "refresh", opts...)
			if err != nil {
				return err
			}
			_, err = src.Token()
			return err
		},
		"token exchange": func(t *testing.T, opts ...oauth2client.Option) error {
			ex, err := oauth2client.NewExchanger(insecureURL, "svc", "s3cr3t", opts...)
			if err != nil {
				return err
			}
			_, err = ex.Exchange(t.Context(), oauth2client.ExchangeRequest{SubjectToken: "subject"})
			return err
		},
		"revocation": func(t *testing.T, opts ...oauth2client.Option) error {
			r, err := oauth2client.NewRevoker(insecureURL, "svc", "s3cr3t", opts...)
			if err != nil {
				return err
			}
			return r.Revoke(t.Context(), "token")
		},
	}

	for name, run := range flows {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client, sent := recordingClient()
			err := run(t, oauth2client.WithHTTPClient(client))
			require.ErrorIs(t, err, oauth2client.ErrInsecureEndpoint)
			require.Zero(t, sent.Load(), "nothing may be sent to a plain-http endpoint")

			client, sent = recordingClient()
			require.NoError(t, run(t, oauth2client.WithHTTPClient(client), oauth2client.WithInsecureHTTP()))
			require.Equal(t, int32(1), sent.Load())
		})
	}
}

func TestFromDiscoveryRejectsInsecureEndpoint(t *testing.T) {
	t.Parallel()
	const insecure = staticEndpoint("http://idp.example/token")

	_, err := oauth2client.ClientCredentialsFromDiscovery(t.Context(), insecure, "svc", "s3cr3t")
	require.ErrorIs(t, err, oauth2client.ErrInsecureEndpoint)
	_, err = oauth2client.NewExchangerFromDiscovery(insecure, "svc", "s3cr3t")
	require.ErrorIs(t, err, oauth2client.ErrInsecureEndpoint)

	_, err = oauth2client.ClientCredentialsFromDiscovery(t.Context(), insecure, "svc", "s3cr3t", oauth2client.WithInsecureHTTP())
	require.NoError(t, err)
	_, err = oauth2client.NewExchangerFromDiscovery(insecure, "svc", "s3cr3t", oauth2client.WithInsecureHTTP())
	require.NoError(t, err)

	// A token endpoint on another host than the issuer stays valid over https.
	_, err = oauth2client.NewExchangerFromDiscovery(staticEndpoint("https://tokens.idp.example/token"), "svc", "s3cr3t")
	require.NoError(t, err)
}

func TestValidateEndpointURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		url           string
		allowInsecure bool
		wantErr       bool
	}{
		{"https", "https://idp.example/token", false, false},
		{"http on localhost", "http://localhost:8080/token", false, false},
		{"http on loopback IPv4", "http://127.0.0.1:8080/token", false, false},
		{"http on loopback IPv6", "http://[::1]:8080/token", false, false},
		{"http on another host", "http://idp.example/token", false, true},
		{"http on a host that only starts with localhost", "http://localhost.evil.example/token", false, true},
		{"http allowed explicitly", "http://idp.example/token", true, false},
		{"other scheme", "ftp://idp.example/token", false, true},
		{"other scheme is not unlocked by the insecure flag", "ftp://idp.example/token", true, true},
		{"relative", "/token", false, true},
		{"relative with the insecure flag", "/token", true, true},
		{"unparsable", "http://idp.example/%zz", false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := oauth2client.ValidateEndpointURL(tc.url, tc.allowInsecure)
			if tc.wantErr {
				require.ErrorIs(t, err, oauth2client.ErrInsecureEndpoint)
				return
			}
			require.NoError(t, err)
		})
	}
}

// The x/oauth2 RetrieveError text embeds the whole IdP response body; neither
// the log line nor the returned error may carry any of it.
func TestLoggedFetchErrorOmitsResponseBody(t *testing.T) {
	t.Parallel()
	const tail = "TAIL-OF-A-LONG-BODY"
	body := strings.Repeat("x", 1024) + tail

	flows := map[string]func(t *testing.T, tokenURL string, logger *slog.Logger) error{
		"client credentials": func(t *testing.T, tokenURL string, logger *slog.Logger) error {
			_, err := mustClientCredentials(t, tokenURL, "svc", "s3cr3t",
				oauth2client.WithLogger(logger), oauth2client.WithAuthStyle(oauth2.AuthStyleInHeader)).Token()
			return err
		},
		"authorization code": func(t *testing.T, tokenURL string, logger *slog.Logger) error {
			ac, err := oauth2client.NewAuthCode(oauth2.Endpoint{TokenURL: tokenURL, AuthStyle: oauth2.AuthStyleInHeader},
				"svc", "s3cr3t", "https://app.example/cb", oauth2client.WithLogger(logger))
			require.NoError(t, err)
			_, _, err = ac.Exchange(t.Context(), "code")
			return err
		},
	}

	for name, run := range flows {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(body))
			}))
			t.Cleanup(srv.Close)
			var logs bytes.Buffer

			err := run(t, srv.URL, slog.New(slog.NewTextHandler(&logs, nil)))

			require.Error(t, err)
			require.NotContains(t, err.Error(), "xxxx")
			require.Contains(t, err.Error(), "response body of 1043 bytes")
			re, ok := errors.AsType[*oauth2.RetrieveError](err)
			require.True(t, ok, "the x/oauth2 error stays reachable")
			require.Contains(t, string(re.Body), tail)
			require.Contains(t, logs.String(), "status 400")
			require.NotContains(t, logs.String(), "xxxx")
		})
	}
}
