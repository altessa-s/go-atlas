// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
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
		time.Sleep(defaultJWKSUnknownKidRefreshInterval)
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

func newUnknownKidTestProvider(t *testing.T, transport testhelpers.RoundTripFunc) *Provider {
	t.Helper()
	provider := &Provider{keys: jwksKeySource{
		backgroundCtx: t.Context(),
		client:        &http.Client{Transport: transport},
		info:          &discoveryInfo{JwksURL: "https://idp.example/jwks"},
		metrics:       newOIDCMetrics(nil),
	}}
	p := &provider.keys
	p.jwksRefreshTask = cycle.New(p.refreshJWKSInternal)
	require.NoError(t, p.initializeJWKS(t.Context()))
	return provider
}

func unknownKidResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}
}
