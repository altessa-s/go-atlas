// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client_test

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/oauth2client"

	"golang.org/x/oauth2"
)

func requestBoundaryError(t testing.TB, grant, endpoint string, client *http.Client, logs *bytes.Buffer) error {
	t.Helper()
	if grant == "native" {
		_, err := mustClientCredentials(t, endpoint, "svc", clientSecret,
			oauth2client.WithClientCredentialsHTTPClient(client),
			oauth2client.WithClientCredentialsLogger(slog.New(slog.NewJSONHandler(logs, nil))),
			oauth2client.WithClientCredentialsAuthStyle(oauth2.AuthStyleInHeader)).Token()
		return err
	}
	_, err := mustExchanger(t, endpoint, "svc", clientSecret,
		oauth2client.WithExchangerHTTPClient(client),
		oauth2client.WithExchangerLogger(slog.New(slog.NewJSONHandler(logs, nil))),
		oauth2client.WithExchangerAuthStyle(oauth2.AuthStyleInHeader)).
		Exchange(t.Context(), oauth2client.ExchangeRequest{SubjectToken: bearerToken})
	return err
}

func TestZeroStatusResponseWithholdsMalformedTrailer(t *testing.T) {
	t.Parallel()
	for _, grant := range []string{"native", "form"} {
		t.Run(grant, func(t *testing.T) {
			t.Parallel()
			const secret = "private-trailer-secret"
			written := make(chan error, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				conn, _, err := http.NewResponseController(w).Hijack()
				if err != nil {
					written <- err
					return
				}
				defer conn.Close()
				_, err = io.WriteString(conn, "HTTP/1.1 000\r\nContent-Type: application/json\r\n"+
					"Transfer-Encoding: chunked\r\nConnection: close\r\n\r\n1\r\nx\r\n0\r\n"+secret+"\r\n\r\n")
				written <- err
			}))
			t.Cleanup(server.Close)
			var logs bytes.Buffer
			err := requestBoundaryError(t, grant, server.URL, server.Client(), &logs)
			require.NoError(t, <-written)
			require.Error(t, err)
			require.NotContains(t, err.Error(), secret)
			require.Contains(t, err.Error(), "status 0")
			require.NotEmpty(t, logs.String())
			require.NotContains(t, logs.String(), secret)
		})
	}
}

func TestFollowedRedirectWithholdsLocationOnConnectionFailure(t *testing.T) {
	t.Parallel()
	for _, grant := range []string{"native", "form"} {
		t.Run(grant, func(t *testing.T) {
			t.Parallel()
			const secret = "private-location-secret"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Location", "/"+secret)
				w.WriteHeader(http.StatusTemporaryRedirect)
			}))
			t.Cleanup(server.Close)
			transport := &http.Transport{DisableKeepAlives: true}
			t.Cleanup(transport.CloseIdleConnections)
			var closeErr error
			hops := 0
			client := &http.Client{
				Transport: transport,
				CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
					hops++
					closeErr = server.Listener.Close()
					return nil
				},
			}
			var logs bytes.Buffer
			err := requestBoundaryError(t, grant, server.URL, client, &logs)
			require.NoError(t, closeErr)
			require.Equal(t, 1, hops)
			require.ErrorIs(t, err, syscall.ECONNREFUSED)
			require.NotContains(t, err.Error(), secret)
			require.NotEmpty(t, logs.String())
			require.NotContains(t, logs.String(), secret)
		})
	}
}
