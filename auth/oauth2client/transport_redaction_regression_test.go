// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

// White-box coverage exercises the runtime guard independently of constructor validation.
func TestTransportErrorsRetainText(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.NotFoundHandler())
	endpoint := server.URL
	server.Close()
	for _, tc := range []struct {
		name, endpoint string
		cause          error
	}{
		{"connection refused", endpoint, syscall.ECONNREFUSED},
		{"insecure guard", "http://idp.example/token", ErrInsecureEndpoint},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client := httpsOnly(&http.Client{})
			ctx, metadata := withResponseClient(t.Context(), client)
			config := clientcredentials.Config{TokenURL: tc.endpoint, AuthStyle: oauth2.AuthStyleInHeader}
			_, original := config.TokenSource(ctx).Token()
			require.ErrorIs(t, original, tc.cause)
			require.Zero(t, metadata.status.Load())
			redacted := metadata.redact(original)
			require.Same(t, original, redacted)
			require.Equal(t, original.Error(), redacted.Error())
			require.ErrorIs(t, redacted, tc.cause)
			var requestError *url.Error
			require.ErrorAs(t, redacted, &requestError)
			_, _, err := doForm(t.Context(), client, tc.endpoint, nil, "", "")
			require.ErrorIs(t, err, tc.cause)
			require.Equal(t, original.Error(), err.Error())
		})
	}
}

func TestReceivedErrorsStillWithheld(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusOK, http.StatusBadRequest} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"access_token":"x","expires_in":"private-response"}`))
			}))
			defer server.Close()
			ctx, metadata := withResponseClient(t.Context(), server.Client())
			config := clientcredentials.Config{TokenURL: server.URL, AuthStyle: oauth2.AuthStyleInHeader}
			_, original := config.TokenSource(ctx).Token()
			require.Error(t, original)
			err := metadata.redact(original)
			require.NotContains(t, err.Error(), "private-response")
			require.ErrorIs(t, err, original)
			require.NotNil(t, errors.Unwrap(err))
			if status == http.StatusBadRequest {
				var retrieved *oauth2.RetrieveError
				require.ErrorAs(t, err, &retrieved)
			}
		})
	}
}
