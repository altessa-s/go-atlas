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
	"golang.org/x/oauth2"

	"github.com/altessa-s/go-atlas/auth/oauth2client"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
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
			opts := []oauth2client.Option{
				oauth2client.WithHTTPClient(client),
				oauth2client.WithAuthStyle(oauth2.AuthStyleInHeader),
				oauth2client.WithLogger(slog.New(slog.NewJSONHandler(&logs, nil))),
			}
			const endpoint = "https://idp.example/token"
			var source oauth2.TokenSource
			var err error
			if grant == "refresh" {
				source, err = oauth2client.Refresh(t.Context(), endpoint, "client", secret, "refresh", opts...)
			} else {
				source, err = oauth2client.ClientCredentials(t.Context(), endpoint, "client", secret, opts...)
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
