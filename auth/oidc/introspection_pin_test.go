// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// pinCache is a JSON round-tripping in-memory Cacher.
type pinCache struct {
	mu sync.Mutex
	m  map[string][]byte
}

func (c *pinCache) Save(_ context.Context, key string, value any, _ ...time.Duration) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[string][]byte{}
	}
	c.m[key] = b
	return nil
}

func (c *pinCache) Get(_ context.Context, key string, value any) error {
	c.mu.Lock()
	b, ok := c.m[key]
	c.mu.Unlock()
	if !ok {
		return errors.New("miss")
	}
	return json.Unmarshal(b, value)
}

type pinErrReader struct{}

func (pinErrReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func TestIntrospectToken_Pin(t *testing.T) {
	t.Parallel()

	reply := func(status int, body io.Reader, calls *atomic.Int32) http.RoundTripper {
		return testhelpers.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
			if calls != nil {
				calls.Add(1)
			}
			return &http.Response{StatusCode: status, Body: io.NopCloser(body), Request: r, Header: http.Header{}}, nil
		})
	}

	t.Run("configuration errors", func(t *testing.T) {
		t.Parallel()
		p := newRevocationTestProvider(t, reply(http.StatusOK, strings.NewReader("{}"), nil), options{})
		p.discoveryInfo = &discoveryInfo{}
		_, err := p.IntrospectToken(t.Context(), "tok")
		require.ErrorIs(t, err, ErrIntrospection)

		p = newRevocationTestProvider(t, reply(http.StatusOK, strings.NewReader("{}"), nil), options{})
		p.opts.introspectionClientID = ""
		_, err = p.IntrospectToken(t.Context(), "tok")
		require.ErrorIs(t, err, ErrIntrospection)

		p = newRevocationTestProvider(t, reply(http.StatusOK, strings.NewReader("{}"), nil), options{})
		p.discoveryInfo = &discoveryInfo{IntrospectionURL: "://bad"}
		_, err = p.IntrospectToken(t.Context(), "tok")
		require.ErrorIs(t, err, ErrIntrospection)
	})

	t.Run("response errors", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name   string
			status int
			body   io.Reader
		}{
			{"status", http.StatusBadGateway, strings.NewReader("down")},
			{"read", http.StatusOK, pinErrReader{}},
			{"json", http.StatusOK, strings.NewReader("{")},
		} {
			p := newRevocationTestProvider(t, reply(tc.status, tc.body, nil), options{})
			_, err := p.IntrospectToken(t.Context(), "tok")
			require.ErrorIs(t, err, ErrIntrospection, tc.name)
		}
	})

	t.Run("active reply is cached", func(t *testing.T) {
		t.Parallel()
		var calls atomic.Int32
		p := newRevocationTestProvider(t, reply(http.StatusOK, strings.NewReader(`{"active":true,"sub":"u"}`), &calls),
			options{activeTokensCacheKeyPrefix: "a:", revokedTokensCacheKeyPrefix: "r:"})
		p.tokenCache = &pinCache{}

		for range 2 {
			resp, err := p.IntrospectToken(t.Context(), "tok")
			require.NoError(t, err)
			require.True(t, resp.Active)
			require.Equal(t, "u", resp.Sub)
		}
		require.Equal(t, int32(1), calls.Load())
	})

	t.Run("inactive reply is cached and marked revoked", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			itemType string
			body     string
			marked   []string
		}{
			{DefaultRevocationItemType, `{"active":false}`, []string{"tok"}},
			{RevocationItemTypeJTI, `{"active":false,"jti":"j1"}`, []string{"j1"}},
			{RevocationItemTypeJTI, `{"active":false}`, nil},
			{RevocationItemTypeKID, `{"active":false,"jti":"j1"}`, nil},
		} {
			var calls atomic.Int32
			p := newRevocationTestProvider(t, reply(http.StatusOK, strings.NewReader(tc.body), &calls),
				options{activeTokensCacheKeyPrefix: "a:", revokedTokensCacheKeyPrefix: "r:", revocationItemType: tc.itemType})
			p.tokenCache = &pinCache{}
			storage := newTrackingRevocationStorage()
			p.revocationStorage = storage

			for range 2 {
				resp, err := p.IntrospectToken(t.Context(), "tok")
				require.NoError(t, err)
				require.False(t, resp.Active)
			}
			require.Equal(t, int32(1), calls.Load(), tc.itemType)
			var marked []string
			for item := range storage.revoked {
				marked = append(marked, item)
			}
			require.ElementsMatch(t, tc.marked, marked, tc.itemType)
		}
	})

	t.Run("an expired inactive reply is not cached", func(t *testing.T) {
		t.Parallel()
		var calls atomic.Int32
		body := `{"active":false,"exp":1}`
		p := newRevocationTestProvider(t, testhelpers.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls.Add(1)
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Request: r, Header: http.Header{}}, nil
		}), options{activeTokensCacheKeyPrefix: "a:", revokedTokensCacheKeyPrefix: "r:"})
		p.tokenCache = &pinCache{}
		for range 2 {
			_, err := p.IntrospectToken(t.Context(), "tok")
			require.NoError(t, err)
		}
		require.Equal(t, int32(2), calls.Load())
	})
}
