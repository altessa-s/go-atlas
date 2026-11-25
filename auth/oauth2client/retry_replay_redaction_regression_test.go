// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client_test

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/oauth2client"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	"golang.org/x/oauth2"
)

func TestRetryBodyReplayErrorRedacted(t *testing.T) {
	t.Parallel()
	retrieved := &oauth2.RetrieveError{
		Response:  &http.Response{StatusCode: http.StatusBadRequest},
		Body:      []byte(`{"error":"invalid_grant","error_description":"private-replay-secret"}`),
		ErrorCode: "invalid_grant", ErrorDescription: "private-replay-secret",
	}
	replayErr := fmt.Errorf("replay request: %w", retrieved)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://idp.example/token", strings.NewReader("payload"))
	require.NoError(t, err)
	req.GetBody = func() (io.ReadCloser, error) { return nil, replayErr }
	attempts := 0
	transport := oauth2client.RetryTransport(testhelpers.RoundTripFunc(func(req *http.Request) (*http.Response, error) {
		attempts++
		_ = req.Body.Close()
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: http.NoBody}, nil
	}), oauth2client.WithTransportAttempts(1), oauth2client.WithTransportBackoff(time.Millisecond, time.Millisecond))
	resp, err := transport.RoundTrip(req)
	require.Nil(t, resp)
	require.ErrorIs(t, err, replayErr)
	var cause *oauth2.RetrieveError
	require.ErrorAs(t, err, &cause)
	require.Same(t, retrieved, cause)
	require.NotContains(t, err.Error(), "private-replay-secret")
	require.Equal(t, 1, attempts)
}
