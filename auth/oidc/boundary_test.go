// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	"golang.org/x/oauth2"
)

func TestEndpointErrorsHideCredentials(t *testing.T) {
	t.Parallel()
	const query = "private-query-value"
	const echoed = "private-body-value"
	for _, failure := range []string{"status", "transport", "deadline", "body", "request"} {
		t.Run(failure, func(t *testing.T) {
			t.Parallel()
			endpoint := "https://idp.example/endpoint?credential=" + query
			if failure == "request" {
				endpoint += "\n"
			}
			cause := context.Canceled
			if failure == "deadline" {
				cause = context.DeadlineExceeded
			}
			rt := testhelpers.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
				if failure == "transport" || failure == "deadline" {
					return nil, fmt.Errorf("%s: %w", echoed, cause)
				}
				resp := jsonResponse(r, echoed)
				if failure == "status" {
					resp.StatusCode = http.StatusUnauthorized
					resp.Status = "401 " + echoed
				} else {
					resp.Body = &testhelpers.MockReadCloser{Reader: iotest.ErrReader(fmt.Errorf("%s: %w", echoed, cause))}
				}
				return resp, nil
			})
			p := newRevocationTestProvider(t, rt, options{})
			p.discovery.info.IntrospectionURL = endpoint
			p.discovery.info.UserInfoURL = endpoint
			var logs bytes.Buffer
			p.revocation.logger = slog.New(slog.NewTextHandler(&logs, nil))
			check := func(t *testing.T, err, kind error) {
				t.Helper()
				require.Error(t, err)
				require.NotContains(t, err.Error(), query)
				require.NotContains(t, err.Error(), echoed)
				require.ErrorIs(t, err, kind)
				require.Nil(t, errors.Unwrap(err))
				_, exposed := errors.AsType[*url.Error](err)
				require.False(t, exposed)
				if failure == "transport" || failure == "deadline" || failure == "body" {
					require.ErrorIs(t, err, cause)
				}
				if failure == "status" {
					require.Contains(t, err.Error(), "401")
				}
			}
			t.Run("introspection", func(t *testing.T) {
				_, err := p.IntrospectToken(t.Context(), echoed)
				check(t, err, ErrIntrospection)
			})
			t.Run("userinfo", func(t *testing.T) {
				_, err := p.UserInfo(t.Context(), oauth2.StaticTokenSource(&oauth2.Token{AccessToken: echoed}))
				check(t, err, ErrTokenInvalid)
			})
			t.Run("loader", func(t *testing.T) {
				loader := &URLRevocationLoader{URL: endpoint, Client: p.discovery.client}
				var loadErr error
				for _, err := range loader.StreamValues(t.Context()) {
					loadErr = err
				}
				check(t, loadErr, ErrRevocationLoadFailed)
				require.ErrorIs(t, loadErr, ErrRevocationCheck)
			})
			for _, open := range []bool{false, true} {
				p.revocation.introspectionFailOpen = open
				err := p.revocation.checkIntrospection(t.Context(), echoed)
				if open {
					require.NoError(t, err)
				} else {
					check(t, err, ErrIntrospection)
				}
				require.NotEmpty(t, logs.String())
				require.NotContains(t, logs.String(), query)
				require.NotContains(t, logs.String(), echoed)
			}
		})
	}
}

func TestStorageAndValidatorDiagnostics(t *testing.T) {
	t.Parallel()
	const secret = "private-storage-value"
	cause := errors.New(secret)
	for _, open := range []bool{false, true} {
		p := newLocalRevocationProvider(t, "token", errRevocationStorage{err: cause})
		var logs bytes.Buffer
		p.revocation.logger = slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
		p.revocation.failOpen = open
		err := p.revocation.checkLocalRevocation(t.Context(), secret, nil, nil)
		if open {
			require.NoError(t, err)
		} else {
			require.ErrorIs(t, err, ErrRevocationCheck)
			require.ErrorIs(t, err, cause)
			require.NotContains(t, err.Error(), secret)
			require.Nil(t, errors.Unwrap(err))
		}
		require.NotContains(t, logs.String(), secret)
	}
	for _, cause := range []error{fmt.Errorf("%w: %s", ErrTokenInvalid, secret), cause} {
		var logs bytes.Buffer
		v := NewValidator(&fakeVerifier{err: cause}, WithValidatorLogger(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))))
		_, err := v.ValidateToken(t.Context(), secret)
		require.ErrorIs(t, err, cause)
		require.NotEmpty(t, logs.String())
		require.NotContains(t, logs.String(), secret)
	}
}

type countingIntrospectionCache struct {
	*memCacher
	active, revoked int
}

func (c *countingIntrospectionCache) Save(ctx context.Context, key string, value any, ttl ...time.Duration) error {
	switch {
	case strings.HasPrefix(key, DefaultActiveTokensCacheKeyPrefix):
		c.active++
	case strings.HasPrefix(key, DefaultRevokedTokensCacheKeyPrefix):
		c.revoked++
	}
	return c.memCacher.Save(ctx, key, value, ttl...)
}

type countingMarks struct {
	syncStorage
	marks int
}

func (s *countingMarks) MarkRevoked(context.Context, string, time.Duration) error {
	s.marks++
	return nil
}

func TestIntrospectionRequiresBooleanActiveBeforeWrites(t *testing.T) {
	t.Parallel()
	idp := newTestIdP(t)
	signed := idp.sign(t, idp.claims(nil))
	for _, body := range []string{`{}`, `null`, `{"active":null}`, `{"active":"false"}`, `[]`, `{"ACTIVE":true}`, `{"active":false}`} {
		for _, open := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/open=%t", body, open), func(t *testing.T) {
				t.Parallel()
				cache := &countingIntrospectionCache{memCacher: newMemCacher()}
				storage := &countingMarks{}
				opts := []Option{WithTokenCache(cache), WithRevocationStorage(storage), WithIntrospection("id", "secret")}
				if open {
					opts = append(opts, WithIntrospectionFailOpen())
				}
				p := idp.newProvider(t, opts...)
				p.discovery.client.Transport = testhelpers.RoundTripFunc(func(r *http.Request) (*http.Response, error) { return jsonResponse(r, body), nil })
				resp, directErr := p.IntrospectToken(t.Context(), "direct")
				_, err := p.ValidateToken(t.Context(), signed)
				require.Zero(t, cache.active)
				if body == `{"active":false}` {
					require.NoError(t, directErr)
					require.False(t, resp.Active)
					require.ErrorIs(t, err, ErrTokenRevoked)
					require.Equal(t, 2, cache.revoked)
					require.Equal(t, 2, storage.marks)
					return
				}
				require.Zero(t, cache.revoked)
				require.Zero(t, storage.marks)
				require.ErrorIs(t, directErr, ErrIntrospection)
				if open {
					require.NoError(t, err)
				} else {
					require.ErrorIs(t, err, ErrIntrospection)
				}
			})
		}
	}
}

func TestValidationDoesNotLogIntrospectionCredentials(t *testing.T) {
	t.Parallel()
	idp := newTestIdP(t)
	signed := idp.sign(t, idp.claims(nil))
	const credential = "private-query-value"
	for _, failure := range []string{"echo", "transport"} {
		for _, open := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/open=%t", failure, open), func(t *testing.T) {
				t.Parallel()
				var logs bytes.Buffer
				logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
				opts := []Option{WithLogger(logger), WithIntrospection("id", "secret")}
				if open {
					opts = append(opts, WithIntrospectionFailOpen())
				}
				p := idp.newProvider(t, opts...)
				p.discovery.info.IntrospectionURL += "?credential=" + credential
				p.discovery.client.Transport = testhelpers.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
					if failure == "transport" {
						return nil, context.DeadlineExceeded
					}
					resp := jsonResponse(r, signed)
					resp.StatusCode = http.StatusForbidden
					return resp, nil
				})
				v := NewValidator(p, WithValidatorLogger(logger))
				_, err := v.ValidateToken(t.Context(), signed)
				require.NotContains(t, logs.String(), credential)
				require.NotContains(t, logs.String(), signed)
				if open {
					require.NoError(t, err)
					return
				}
				require.ErrorIs(t, err, ErrIntrospection)
				require.NotContains(t, err.Error(), credential)
				require.NotContains(t, err.Error(), signed)
				if failure == "transport" {
					require.ErrorIs(t, err, context.DeadlineExceeded)
				}
			})
		}
	}
}

func TestRevocationFetchStatusSurvivesSync(t *testing.T) {
	t.Parallel()
	idp := newTestIdP(t)
	loader := &URLRevocationLoader{URL: idp.srv.URL + "/revoked?credential=private-query-value"}
	opts := []Option{WithRevocationFilter(newRebuildableFilter()), WithRevocationLoader(loader)}
	p := idp.newProvider(t, opts...)
	idp.revokedStatus.Store(http.StatusUnauthorized)
	for _, err := range []error{p.revocation.scheduledRevocationSync(t.Context()), p.revocation.initialRevocationSync(t.Context())} {
		require.ErrorIs(t, err, ErrRevocationCheck)
		require.ErrorIs(t, err, ErrRevocationLoadFailed)
		require.Contains(t, err.Error(), "401")
		require.NotContains(t, err.Error(), "private-query-value")
		require.Nil(t, errors.Unwrap(err))
	}
}
