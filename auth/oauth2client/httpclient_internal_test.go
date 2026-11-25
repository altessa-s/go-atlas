// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"golang.org/x/oauth2"
)

// White-box: the client a flow ends up with is not observable from outside.
func TestOptionsClient(t *testing.T) {
	t.Parallel()

	t.Run("default client has a timeout and stops at redirects", func(t *testing.T) {
		t.Parallel()
		c := endpointClient(nil, false)
		require.Equal(t, DefaultHTTPTimeout, c.Timeout)
		require.ErrorIs(t, c.CheckRedirect(nil, nil), http.ErrUseLastResponse)
		require.IsType(t, &httpsOnlyTransport{}, c.Transport)
	})

	t.Run("x/oauth2 reads the same client from the context", func(t *testing.T) {
		t.Parallel()
		c := endpointClient(nil, false)
		got, ok := withHTTPClient(t.Context(), c).Value(oauth2.HTTPClient).(*http.Client)
		require.True(t, ok)
		require.Same(t, c, got)
	})

	t.Run("injected client keeps its settings behind the https guard", func(t *testing.T) {
		t.Parallel()
		injected := &http.Client{Timeout: 7, Transport: http.DefaultTransport}
		c := endpointClient(injected, false)
		require.NotSame(t, injected, c, "the injected client itself must not be modified")
		require.Equal(t, injected.Timeout, c.Timeout)
		guard, ok := c.Transport.(*httpsOnlyTransport)
		require.True(t, ok)
		require.Same(t, http.DefaultTransport, guard.base)
		require.Same(t, http.DefaultTransport, injected.Transport)
	})

	t.Run("insecure option removes the https guard only", func(t *testing.T) {
		t.Parallel()
		injected := &http.Client{Timeout: 7, Transport: http.DefaultTransport}
		got := endpointClient(injected, true)
		require.NotSame(t, injected, got)
		require.Same(t, http.DefaultTransport, got.Transport, "no https guard on the transport")
		require.ErrorIs(t, got.CheckRedirect(nil, nil), http.ErrUseLastResponse, "the redirect guard stays")
		require.Nil(t, injected.CheckRedirect, "the injected client itself must not be modified")

		c := endpointClient(nil, true)
		require.Equal(t, DefaultHTTPTimeout, c.Timeout)
		require.ErrorIs(t, c.CheckRedirect(nil, nil), http.ErrUseLastResponse)
	})
}

func TestLogError(t *testing.T) {
	t.Parallel()
	re := &oauth2.RetrieveError{
		Response:  &http.Response{StatusCode: http.StatusUnauthorized},
		Body:      []byte(`{"error":"invalid_client"}`),
		ErrorCode: "invalid_client",
	}

	const want = `identity provider returned status 401: error "invalid_client"`
	require.Equal(t, want, logError(re))
	require.Equal(t, want, logError(fmt.Errorf("device access token: %w", re)), "a wrapped RetrieveError is redacted too")
	require.Contains(t, logError(&oauth2.RetrieveError{}), "status 0", "a missing response must not panic")
	require.Equal(t, "boom", logError(errors.New("boom")), "other errors are logged as is")
}
