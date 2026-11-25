// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/oauth2client"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	"golang.org/x/oauth2"
)

func TestCancellationWithRetrieveErrorRedactsResponse(t *testing.T) {
	t.Parallel()
	for _, grant := range []string{"client credentials", "refresh"} {
		t.Run(grant, func(t *testing.T) {
			t.Parallel()
			const secret = "joined-cancellation-secret"
			retrieved := &oauth2.RetrieveError{
				Response:         &http.Response{StatusCode: http.StatusUnauthorized},
				Body:             []byte(secret),
				ErrorCode:        "invalid_client",
				ErrorDescription: secret,
			}
			client := &http.Client{Transport: testhelpers.RoundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.Join(context.DeadlineExceeded, retrieved)
			})}
			var logs bytes.Buffer
			const endpoint = "https://idp.example/token"
			var source oauth2.TokenSource
			var err error
			if grant == "refresh" {
				source, err = oauth2client.Refresh(t.Context(), endpoint, "client", secret, "refresh",
					oauth2client.WithRefreshHTTPClient(client),
					oauth2client.WithRefreshLogger(slog.New(slog.NewJSONHandler(&logs, nil))),
					oauth2client.WithRefreshAuthStyle(oauth2.AuthStyleInHeader))
			} else {
				source, err = oauth2client.ClientCredentials(t.Context(), endpoint, "client", secret,
					oauth2client.WithClientCredentialsHTTPClient(client),
					oauth2client.WithClientCredentialsLogger(slog.New(slog.NewJSONHandler(&logs, nil))),
					oauth2client.WithClientCredentialsAuthStyle(oauth2.AuthStyleInHeader))
			}
			require.NoError(t, err)
			_, err = source.Token()
			require.ErrorIs(t, err, context.DeadlineExceeded)
			require.ErrorIs(t, err, retrieved)
			require.NotEmpty(t, logs.String())
			require.NotContains(t, logs.String(), secret)
			require.NotContains(t, err.Error(), secret)
		})
	}
}
