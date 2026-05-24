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
// pre- and post-verification revocation helpers with no introspection and
// no JWKS — only the revocation plumbing is exercised.
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

// TestProvider_checkTokenRevocation_SkipsJTIPreVerify proves the pre-verify
// path never queries the storage by jti — that lookup is deferred until
// claims have been signature-verified.
func TestProvider_checkTokenRevocation_SkipsJTIPreVerify(t *testing.T) {
	t.Parallel()

	storage := newTrackingRevocationStorage("revoked-jti")
	p := newLocalRevocationProvider(t, RevocationItemTypeJTI, storage)

	// Craft a token whose unverified jti matches the revoked entry. Before
	// the fix this would have been queried and the token rejected without
	// signature verification.
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"jti": "revoked-jti"})
	raw, err := tok.SignedString([]byte("test-secret"))
	require.NoError(t, err)

	require.NoError(t, p.checkTokenRevocation(t.Context(), raw),
		"pre-verify path must not consult revocation storage by jti from an unverified token")
	require.Empty(t, storage.lookups, "no storage lookup must be performed before signature verification")
}

// TestProvider_checkTokenRevocation_SkipsKIDPreVerify mirrors the jti test
// for kid-typed revocation storage.
func TestProvider_checkTokenRevocation_SkipsKIDPreVerify(t *testing.T) {
	t.Parallel()

	storage := newTrackingRevocationStorage("revoked-kid")
	p := newLocalRevocationProvider(t, RevocationItemTypeKID, storage)

	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": "anyone"})
	tok.Header["kid"] = "revoked-kid"
	raw, err := tok.SignedString([]byte("test-secret"))
	require.NoError(t, err)

	require.NoError(t, p.checkTokenRevocation(t.Context(), raw),
		"pre-verify path must not consult revocation storage by kid from an unverified token header")
	require.Empty(t, storage.lookups, "no storage lookup must be performed before signature verification")
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

// TestProvider_checkTokenRevocation_FullTokenStillRunsPreVerify confirms
// the pre-verify path is still used when revocation is keyed on the full
// token — that case is safe because the token string itself is the key.
func TestProvider_checkTokenRevocation_FullTokenStillRunsPreVerify(t *testing.T) {
	t.Parallel()

	storage := newTrackingRevocationStorage("the-full-token")
	p := newLocalRevocationProvider(t, DefaultRevocationItemType, storage)

	err := p.checkTokenRevocation(t.Context(), "the-full-token")
	require.ErrorIs(t, err, ErrTokenRevoked,
		"full-token revocation must continue to be enforced pre-verify")
	require.Equal(t, []string{"the-full-token"}, storage.lookups)
}
