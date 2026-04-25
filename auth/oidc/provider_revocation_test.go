// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// newRevocationTestProvider builds a Provider wired for direct calls to
// checkTokenRevocation: introspection enabled with mock HTTP transport,
// no token cache, no revocation storage. The returned Provider uses no-op
// metrics.
func newRevocationTestProvider(t *testing.T, rt http.RoundTripper, opts options) *Provider {
	t.Helper()
	opts.introspectionEnabled = true
	opts.introspectionClientID = "client-id"
	opts.introspectionSecret = "client-secret"
	return &Provider{
		opts:          &opts,
		client:        &http.Client{Transport: rt},
		discoveryInfo: &discoveryInfo{IntrospectionURL: "https://issuer/introspect"},
		logger:        slog.New(slog.DiscardHandler),
		metrics:       newOIDCMetrics(nil),
	}
}

func TestProvider_checkTokenRevocation_FailOpenByDefault(t *testing.T) {
	t.Parallel()

	transportErr := errors.New("connection refused")
	rt := testhelpers.RoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, transportErr
	})

	p := newRevocationTestProvider(t, rt, options{})

	require.NoError(t, p.checkTokenRevocation(t.Context(), "any-token"),
		"default (non-strict) introspection failure must be swallowed")
}

func TestProvider_checkTokenRevocation_StrictRejectsOnTransportError(t *testing.T) {
	t.Parallel()

	transportErr := errors.New("connection refused")
	rt := testhelpers.RoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, transportErr
	})

	p := newRevocationTestProvider(t, rt, options{introspectionStrict: true})

	err := p.checkTokenRevocation(t.Context(), "any-token")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrIntrospection,
		"strict mode must surface ErrIntrospection on transport failure")
}

func TestProvider_checkTokenRevocation_StrictRejectsOn5xx(t *testing.T) {
	t.Parallel()

	rt := testhelpers.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Status:     "502 Bad Gateway",
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    r,
		}, nil
	})

	p := newRevocationTestProvider(t, rt, options{introspectionStrict: true})

	err := p.checkTokenRevocation(t.Context(), "any-token")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrIntrospection,
		"strict mode must reject when introspection endpoint returns non-2xx")
}

func TestProvider_checkTokenRevocation_StrictAcceptsActiveToken(t *testing.T) {
	t.Parallel()

	rt := testhelpers.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"active":true}`)),
			Request:    r,
		}, nil
	})

	p := newRevocationTestProvider(t, rt, options{introspectionStrict: true})

	require.NoError(t, p.checkTokenRevocation(t.Context(), "any-token"),
		"strict mode must still accept tokens the IdP confirms as active")
}

func TestProvider_checkTokenRevocation_StrictRejectsInactiveToken(t *testing.T) {
	t.Parallel()

	rt := testhelpers.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"active":false}`)),
			Request:    r,
		}, nil
	})

	p := newRevocationTestProvider(t, rt, options{introspectionStrict: true})

	err := p.checkTokenRevocation(t.Context(), "any-token")
	require.ErrorIs(t, err, ErrTokenRevoked,
		"strict mode must still surface revocation of inactive tokens via ErrTokenRevoked")
}
