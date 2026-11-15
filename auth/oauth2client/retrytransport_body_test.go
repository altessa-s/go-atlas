// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/oauth2client"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func TestRetryTransportClosesBodyBeforeFirstAttempt(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://example.com/token", strings.NewReader("payload"))
	require.NoError(t, err)
	body := &testhelpers.MockReadCloser{Reader: req.Body}
	req.Body = body
	called := false
	transport := oauth2client.RetryTransport(testhelpers.RoundTripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, context.Canceled
	}))
	resp, err := transport.RoundTrip(req)
	require.Nil(t, resp)
	require.ErrorIs(t, err, context.Canceled)
	require.False(t, called)
	require.True(t, body.Closed, "RoundTripper must close the request body even when no attempt is made")
}
