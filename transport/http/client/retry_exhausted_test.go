// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	httpclient "github.com/altessa-s/go-atlas/transport/http/client"
)

// statusServer answers every request with status and counts the hits.
func statusServer(t *testing.T, status int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func doGet(t *testing.T, c *http.Client, ctx context.Context, url string) error {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	require.NoError(t, err)
	resp, err := c.Do(req)
	if err == nil {
		_ = resp.Body.Close()
	}
	return err
}

var fastRetries = httpclient.WithRetryWait(time.Millisecond, time.Millisecond)

// A retryable status still failing after the last retry is a
// RetryExhaustedError wrapping the UnexpectedStatusError of the last reply.
func TestRetryExhaustedStatus(t *testing.T) {
	t.Parallel()

	srv, hits := statusServer(t, http.StatusServiceUnavailable)
	c := httpclient.New(loopback, httpclient.WithRetryMax(2), fastRetries)
	err := doGet(t, c, t.Context(), srv.URL+"/orders?token=secret")

	require.EqualValues(t, 3, hits.Load())
	require.ErrorIs(t, err, httpclient.ErrMaxRetriesExceeded)
	re := httpclient.IsRetryExhaustedError(err)
	require.NotNil(t, re, "%v", err)
	require.Equal(t, 3, re.Attempts)
	require.Equal(t, http.MethodGet, re.Method)
	require.Equal(t, srv.URL+"/orders", re.URL, "the URL omits the query string")
	se := httpclient.IsUnexpectedStatusError(err)
	require.NotNil(t, se)
	require.Equal(t, http.StatusServiceUnavailable, se.Status)
	require.Equal(t, strings.TrimPrefix(srv.URL, "http://"), se.Host)
	require.Equal(t, "/orders?token=secret", se.URI, "the status error keeps the request URI, as for a status not retried")
}

// A custom retry policy that retries a status reports its exhaustion the
// same way.
func TestRetryExhaustedStatusWithPolicy(t *testing.T) {
	t.Parallel()

	srv, hits := statusServer(t, http.StatusInternalServerError)
	retry500 := func(_ context.Context, resp *http.Response, err error) (bool, error) {
		return err != nil || resp.StatusCode == http.StatusInternalServerError, err
	}
	c := httpclient.New(loopback, httpclient.WithRetryMax(1), fastRetries, httpclient.WithRetryPolicyHandler(retry500))
	err := doGet(t, c, t.Context(), srv.URL)
	require.EqualValues(t, 2, hits.Load())
	require.NotNil(t, httpclient.IsRetryExhaustedError(err))
	se := httpclient.IsUnexpectedStatusError(err)
	require.NotNil(t, se)
	require.Equal(t, http.StatusInternalServerError, se.Status)
}

// A transport failure on every attempt is a RetryExhaustedError wrapping the
// last transport error.
func TestRetryExhaustedTransportError(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	reset := &net.OpError{Op: "read", Net: "tcp", Err: errors.New("connection reset by peer")}
	failing := testhelpers.RoundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, reset
	})
	c := httpclient.New(httpclient.WithoutSSRFProtection(), httpclient.WithClient(&http.Client{Transport: failing}),
		httpclient.WithRetryMax(2), fastRetries)
	err := doGet(t, c, t.Context(), "http://upstream.invalid/x")
	require.EqualValues(t, 3, calls.Load())
	re := httpclient.IsRetryExhaustedError(err)
	require.NotNil(t, re, "%v", err)
	require.Equal(t, 3, re.Attempts)
	require.ErrorIs(t, err, reset)
}

// Failures that are not retried, and a context that ends while waiting for a
// retry, are not exhaustion.
func TestRetryNotExhausted(t *testing.T) {
	t.Parallel()

	notFound, hits := statusServer(t, http.StatusNotFound)
	err := doGet(t, httpclient.New(loopback, httpclient.WithRetryMax(2), fastRetries), t.Context(), notFound.URL)
	require.EqualValues(t, 1, hits.Load())
	require.Nil(t, httpclient.IsRetryExhaustedError(err))
	require.NotNil(t, httpclient.IsUnexpectedStatusError(err))

	unavailable, _ := statusServer(t, http.StatusServiceUnavailable)
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	slow := httpclient.New(loopback, httpclient.WithRetryMax(5), httpclient.WithRetryWait(time.Second, time.Second))
	err = doGet(t, slow, ctx, unavailable.URL)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Nil(t, httpclient.IsRetryExhaustedError(err), "an ended context is not exhaustion")
}

// The error handler receives the RetryExhaustedError.
func TestRetryExhaustedErrorHandler(t *testing.T) {
	t.Parallel()

	srv, _ := statusServer(t, http.StatusBadGateway)
	var got error
	handler := func(resp *http.Response, err error, _ int) (*http.Response, error) {
		got = err
		return nil, err
	}
	err := doGet(t, httpclient.New(loopback, httpclient.WithRetryMax(1), fastRetries, httpclient.WithErrorHandler(handler)),
		t.Context(), srv.URL)
	require.Error(t, err)
	require.NotNil(t, httpclient.IsRetryExhaustedError(got))
	require.Equal(t, http.StatusBadGateway, httpclient.IsUnexpectedStatusError(got).Status)
}

// A retryable status exhausted on a redirect hop names the hop's host: the
// hop's request has no Host field.
func TestRetryExhaustedOnRedirectHop(t *testing.T) {
	t.Parallel()

	target, _ := statusServer(t, http.StatusServiceUnavailable)
	redirect := httptest.NewServer(http.RedirectHandler(target.URL+"/to", http.StatusFound))
	t.Cleanup(redirect.Close)
	err := doGet(t, httpclient.New(loopback, httpclient.WithRetryMax(1), fastRetries), t.Context(), redirect.URL)
	se := httpclient.IsUnexpectedStatusError(err)
	require.NotNil(t, se, "%v", err)
	require.Equal(t, strings.TrimPrefix(target.URL, "http://"), se.Host)
	require.Equal(t, "/to", se.URI)
}
