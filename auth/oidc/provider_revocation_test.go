// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// newRevocationTestProvider builds a Provider wired for direct calls to
// checkIntrospection: introspection enabled with mock HTTP transport,
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

func TestProvider_checkIntrospection_FailClosedByDefault(t *testing.T) {
	t.Parallel()

	transportErr := errors.New("connection refused")
	rt := testhelpers.RoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, transportErr
	})

	p := newRevocationTestProvider(t, rt, options{})

	err := p.checkIntrospection(t.Context(), "any-token")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrIntrospection,
		"default (fail-closed) introspection failure must surface ErrIntrospection")
}

func TestProvider_checkIntrospection_FailOpenSwallowsTransportError(t *testing.T) {
	t.Parallel()

	transportErr := errors.New("connection refused")
	rt := testhelpers.RoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, transportErr
	})

	p := newRevocationTestProvider(t, rt, options{introspectionFailOpen: true})

	require.NoError(t, p.checkIntrospection(t.Context(), "any-token"),
		"fail-open mode must swallow introspection transport failures")
}

func TestProvider_checkIntrospection_StrictRejectsOn5xx(t *testing.T) {
	t.Parallel()

	rt := testhelpers.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Status:     "502 Bad Gateway",
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    r,
		}, nil
	})

	p := newRevocationTestProvider(t, rt, options{})

	err := p.checkIntrospection(t.Context(), "any-token")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrIntrospection,
		"strict mode must reject when introspection endpoint returns non-2xx")
}

func TestProvider_checkIntrospection_StrictAcceptsActiveToken(t *testing.T) {
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

	p := newRevocationTestProvider(t, rt, options{})

	require.NoError(t, p.checkIntrospection(t.Context(), "any-token"),
		"strict mode must still accept tokens the IdP confirms as active")
}

func TestProvider_checkIntrospection_StrictRejectsInactiveToken(t *testing.T) {
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

	p := newRevocationTestProvider(t, rt, options{})

	err := p.checkIntrospection(t.Context(), "any-token")
	require.ErrorIs(t, err, ErrTokenRevoked,
		"strict mode must still surface revocation of inactive tokens via ErrTokenRevoked")
}

// trackingRevocationStorage records every IsRevoked lookup key so tests can
// assert exactly which item the provider queried (full token vs jti vs kid).
type trackingRevocationStorage struct {
	revoked map[string]struct{}
	lookups []string
}

func newTrackingRevocationStorage(revoked ...string) *trackingRevocationStorage {
	s := &trackingRevocationStorage{revoked: make(map[string]struct{}, len(revoked))}
	for _, item := range revoked {
		s.revoked[item] = struct{}{}
	}
	return s
}

func (s *trackingRevocationStorage) IsRevoked(_ context.Context, item string) (bool, error) {
	s.lookups = append(s.lookups, item)
	_, ok := s.revoked[item]
	return ok, nil
}

func (s *trackingRevocationStorage) MarkRevoked(_ context.Context, item string, _ time.Duration) error {
	s.revoked[item] = struct{}{}
	return nil
}

func (s *trackingRevocationStorage) Sync(_ context.Context) error { return nil }

// newLocalRevocationProvider builds a Provider wired for direct calls to the
// post-verification revocation helper with no introspection and no JWKS —
// only the revocation plumbing is exercised.
func newLocalRevocationProvider(t *testing.T, itemType string, storage RevocationStorage) *Provider {
	t.Helper()
	return &Provider{
		opts: &options{
			revocationItemType: itemType,
		},
		revocationStorage: storage,
		logger:            slog.New(slog.DiscardHandler),
		metrics:           newOIDCMetrics(nil),
	}
}

// TestProvider_checkTokenRevocationVerified_RejectsRevokedJTI ensures the
// deferred lookup actually triggers once signature-verified claims are in
// hand.
func TestProvider_checkTokenRevocationVerified_RejectsRevokedJTI(t *testing.T) {
	t.Parallel()

	storage := newTrackingRevocationStorage("revoked-jti")
	p := newLocalRevocationProvider(t, RevocationItemTypeJTI, storage)

	claims := jwt.MapClaims{"jti": "revoked-jti"}
	err := p.checkTokenRevocationVerified(t.Context(), "ignored-token", claims, nil)
	require.ErrorIs(t, err, ErrTokenRevoked,
		"post-verify path must reject tokens whose verified jti is in the revocation set")
	require.Equal(t, []string{"revoked-jti"}, storage.lookups)
}

// TestProvider_checkTokenRevocationVerified_RejectsRevokedKID ensures the
// deferred kid lookup honors the verified token header.
func TestProvider_checkTokenRevocationVerified_RejectsRevokedKID(t *testing.T) {
	t.Parallel()

	storage := newTrackingRevocationStorage("revoked-kid")
	p := newLocalRevocationProvider(t, RevocationItemTypeKID, storage)

	header := map[string]any{"kid": "revoked-kid"}
	err := p.checkTokenRevocationVerified(t.Context(), "ignored-token", jwt.MapClaims{}, header)
	require.ErrorIs(t, err, ErrTokenRevoked,
		"post-verify path must reject tokens whose verified kid is in the revocation set")
	require.Equal(t, []string{"revoked-kid"}, storage.lookups)
}

// TestProvider_checkTokenRevocationVerified_FallsBackToFullToken proves the
// post-verify helper degrades gracefully (full-token key) when the
// configured field is missing from claims/header rather than skipping the
// check entirely.
func TestProvider_checkTokenRevocationVerified_FallsBackToFullToken(t *testing.T) {
	t.Parallel()

	storage := newTrackingRevocationStorage("the-full-token")
	p := newLocalRevocationProvider(t, RevocationItemTypeJTI, storage)

	err := p.checkTokenRevocationVerified(t.Context(), "the-full-token", jwt.MapClaims{}, nil)
	require.ErrorIs(t, err, ErrTokenRevoked,
		"missing jti must fall back to the full-token key, not skip the lookup")
	require.Equal(t, []string{"the-full-token"}, storage.lookups)
}

// TestProvider_checkTokenRevocationVerified_FullToken confirms revocation
// keyed on the full token is enforced by the post-verification check.
func TestProvider_checkTokenRevocationVerified_FullToken(t *testing.T) {
	t.Parallel()

	storage := newTrackingRevocationStorage("the-full-token")
	p := newLocalRevocationProvider(t, DefaultRevocationItemType, storage)

	err := p.checkTokenRevocationVerified(t.Context(), "the-full-token", jwt.MapClaims{"jti": "ignored"}, nil)
	require.ErrorIs(t, err, ErrTokenRevoked,
		"full-token revocation must be enforced after verification")
	require.Equal(t, []string{"the-full-token"}, storage.lookups)
}
