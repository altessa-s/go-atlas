// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"crypto/rsa"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/MicahParks/jwkset"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/runtime/cycle"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	authjwt "github.com/altessa-s/go-atlas/auth/jwt"
)

func TestUnknownKidRefreshThrottlesFailedAttempts(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		active := sigJWK(t, "active")
		attempts := 0
		provider := newUnknownKidTestProvider(t, func(*http.Request) (*http.Response, error) {
			attempts++
			if attempts == 1 {
				return unknownKidResponse(http.StatusOK, jwksOf(t, active)), nil
			}
			return unknownKidResponse(http.StatusBadGateway, ""), nil
		})
		p := &provider.keys
		freshness := p.lastJWKSRefresh.Load()
		for i := range 100 {
			_, err := p.keyResolver.ResolveKey(t.Context(),
				authjwt.Header{Kid: fmt.Sprintf("unknown-%d", i), Alg: "RS256"}, nil)
			require.ErrorIs(t, err, jwkset.ErrKeyNotFound)
		}
		require.Equal(t, 2, attempts)
		require.Same(t, freshness, p.lastJWKSRefresh.Load())
		time.Sleep(DefaultJWKSUnknownKidRefreshInterval)
		_, err := p.keyResolver.ResolveKey(t.Context(), authjwt.Header{Kid: "new", Alg: "RS256"}, nil)
		require.ErrorIs(t, err, jwkset.ErrKeyNotFound)
		require.Equal(t, 3, attempts)
		require.Same(t, freshness, p.lastJWKSRefresh.Load())
	})
}

func TestUnknownKidJoinsScheduledRefresh(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		active := sigJWK(t, "active")
		rotated := sigJWK(t, "rotated")
		started, release := make(chan struct{}), make(chan struct{})
		attempts := 0
		provider := newUnknownKidTestProvider(t, func(*http.Request) (*http.Response, error) {
			attempts++
			switch attempts {
			case 1:
				return unknownKidResponse(http.StatusOK, jwksOf(t, active)), nil
			case 2:
				close(started)
				<-release
			}
			return unknownKidResponse(http.StatusOK, jwksOf(t, rotated)), nil
		})
		p := &provider.keys
		scheduled := make(chan error, 1)
		go func() { scheduled <- provider.scheduledJWKSRefresh(t.Context()) }()
		<-started
		resolved := make(chan authjwt.VerificationKey, 1)
		resolveErr := make(chan error, 1)
		go func() {
			key, err := p.keyResolver.ResolveKey(t.Context(), authjwt.Header{Kid: "rotated", Alg: "RS256"}, nil)
			resolved <- key
			resolveErr <- err
		}()
		synctest.Wait()
		beforeRelease := attempts
		close(release)
		require.NoError(t, <-scheduled)
		require.NoError(t, <-resolveErr)
		require.IsType(t, (*rsa.PublicKey)(nil), (<-resolved).Key)
		require.Equal(t, 2, beforeRelease)
		require.Equal(t, 2, attempts)
	})
}

func TestProviderUnknownKidRefreshInterval(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		active := jwksOf(t, sigJWK(t, "active"))
		attempts := 0
		transport := testhelpers.RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path == "/jwks" {
				attempts++
				return unknownKidResponse(http.StatusOK, active), nil
			}
			return unknownKidResponse(http.StatusOK, unknownKidDiscovery), nil
		})
		const interval = 5 * time.Second
		provider, err := NewProvider(t.Context(), "https://idp.example/.well-known/openid-configuration",
			WithHTTPClient(&http.Client{Transport: transport}), WithJWKSUnknownKidRefreshInterval(interval))
		require.NoError(t, err)
		defer provider.Close()

		resolve := func() {
			t.Helper()
			_, err := provider.keys.keyResolver.ResolveKey(t.Context(), authjwt.Header{Kid: "unknown", Alg: "RS256"}, nil)
			require.ErrorIs(t, err, jwkset.ErrKeyNotFound)
		}
		resolve()
		time.Sleep(interval - time.Second)
		resolve()
		require.Equal(t, 2, attempts)
		time.Sleep(time.Second)
		resolve()
		require.Equal(t, 3, attempts)
	})
}

func TestProviderCloseJoinsUnknownKidRefresh(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		active := jwksOf(t, sigJWK(t, "active"))
		started, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
		attempts := 0
		transport := &idleCountingTransport{RoundTripper: testhelpers.RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/jwks" {
				return unknownKidResponse(http.StatusOK, unknownKidDiscovery), nil
			}
			attempts++
			if attempts == 1 {
				return unknownKidResponse(http.StatusOK, active), nil
			}
			close(started)
			<-req.Context().Done()
			close(canceled)
			<-release
			return nil, req.Context().Err()
		})}
		provider, err := NewProvider(t.Context(), "https://idp.example/.well-known/openid-configuration",
			WithHTTPClient(&http.Client{Transport: transport}), WithOwnedHTTPClient())
		require.NoError(t, err)

		resolve := func() error {
			_, err := provider.keys.keyResolver.ResolveKey(t.Context(), authjwt.Header{Kid: "unknown", Alg: "RS256"}, nil)
			return err
		}
		original := make(chan error, 1)
		go func() { original <- resolve() }()
		<-started
		closed := make(chan struct{}, 1)
		go func() {
			provider.Close()
			closed <- struct{}{}
		}()
		<-canceled
		late := make(chan error, 1)
		go func() { late <- resolve() }()
		synctest.Wait()
		clientClosedEarly := transport.closed.Load()
		closeReturnedEarly := len(closed) > 0
		admissionStopped := len(late) == 1
		close(release)
		<-closed
		require.ErrorIs(t, <-original, jwkset.ErrKeyNotFound)
		require.ErrorIs(t, <-late, jwkset.ErrKeyNotFound)
		require.Zero(t, clientClosedEarly)
		require.False(t, closeReturnedEarly)
		require.True(t, admissionStopped)
		require.EqualValues(t, 1, transport.closed.Load())
		require.Equal(t, 2, attempts)
	})
}

func TestScheduledRefreshWaitsAfterJoiningSkippedFlight(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		active := jwksOf(t, sigJWK(t, "active"))
		attempts := 0
		provider := newUnknownKidTestProvider(t, func(*http.Request) (*http.Response, error) {
			attempts++
			return unknownKidResponse(http.StatusOK, active), nil
		})
		started, release := make(chan struct{}), make(chan struct{})
		skipped := make(chan error, 1)
		go func() {
			_, err := provider.keys.refreshFlight.Do(t.Context(), struct{}{}, func(context.Context) (struct{}, error) {
				close(started)
				<-release
				return struct{}{}, errUnknownKidRefreshSkipped
			})
			skipped <- err
		}()
		<-started

		start := time.Now()
		scheduled := make(chan error, 1)
		go func() { scheduled <- provider.scheduledJWKSRefresh(t.Context()) }()
		synctest.Wait()
		close(release)
		require.ErrorIs(t, <-skipped, errUnknownKidRefreshSkipped)
		require.NoError(t, <-scheduled)
		require.Positive(t, time.Since(start))
		require.Equal(t, 2, attempts)
	})
}

const unknownKidDiscovery = `{"issuer":"https://idp.example","jwks_uri":"https://idp.example/jwks",
"id_token_signing_alg_values_supported":["RS256"]}`

func newUnknownKidTestProvider(t *testing.T, transport testhelpers.RoundTripFunc) *Provider {
	t.Helper()
	provider := &Provider{keys: jwksKeySource{
		backgroundCtx:                 t.Context(),
		jwksUnknownKidRefreshInterval: DefaultJWKSUnknownKidRefreshInterval,
		client:                        &http.Client{Transport: transport},
		info:                          &discoveryInfo{JwksURL: "https://idp.example/jwks"},
		metrics:                       newOIDCMetrics(nil),
	}}
	p := &provider.keys
	p.jwksRefreshTask = cycle.New(p.refreshJWKSInternal)
	require.NoError(t, p.initializeJWKS(t.Context()))
	return provider
}

func unknownKidResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}
}
