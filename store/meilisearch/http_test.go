// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package meilisearch_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/store/meilisearch"
)

func TestRedirects_SameOriginOnly(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		host     string
		location string
		allowed  bool
	}{
		{"path", "http://example.com:7700", "/ready", true},
		{"canonical host and default port", "https://EXAMPLE.com", "https://example.com:443/ready", true},
		{"IDNA", "https://bücher.example", "https://xn--bcher-kva.example/ready", true},
		{"IPv6", "http://[0:0:0:0:0:0:0:1]:7700", "http://[::1]:7700/ready", true},
		{"absolute DNS name", "https://EXAMPLE.com.", "https://example.com./ready", true},
		{"other port", "http://example.com:7700", "http://example.com:7701/ready", false},
		{"subdomain", "http://example.com", "http://sub.example.com/ready", false},
		{"downgrade", "https://example.com:7700", "http://example.com:7700/ready", false},
		{"trailing dot changed", "https://example.com.", "https://example.com/ready", false},
		{"Unicode trailing dot added", "https://example.com", "https://example.com。/ready", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var paths []string
			httpClient := &http.Client{Transport: testhelpers.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
				paths = append(paths, r.URL.Path)
				if r.URL.Path == "/health" {
					require.Equal(t, "Bearer secret-marker", r.Header.Get("Authorization"))
					return &http.Response{
						StatusCode: http.StatusFound,
						Header:     http.Header{"Location": {tc.location}},
						Body:       http.NoBody,
					}, nil
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(available))}, nil
			})}

			client, err := meilisearch.New(t.Context(), tc.host,
				meilisearch.WithHTTPClient(httpClient), meilisearch.WithAPIKey("secret-marker"))
			if tc.allowed {
				require.NoError(t, err)
				defer client.Close() //nolint:errcheck // test cleanup
				require.Equal(t, []string{"/health", "/ready"}, paths)
				return
			}
			require.ErrorIs(t, err, meilisearch.ErrHealthCheck)
			require.Equal(t, []string{"/health"}, paths)
		})
	}
}

func TestRedirects_MaxTenHops(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		hops int
		want error
	}{
		{"ten redirects", 10, nil},
		{"eleven redirects", 11, meilisearch.ErrHealthCheck},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			calls := 0
			httpClient := &http.Client{Transport: testhelpers.RoundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				if calls <= tc.hops {
					return &http.Response{
						StatusCode: http.StatusFound,
						Header:     http.Header{"Location": {"/ready"}},
						Body:       http.NoBody,
					}, nil
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(available))}, nil
			})}
			client, err := meilisearch.New(t.Context(), "https://example.com", meilisearch.WithHTTPClient(httpClient))
			require.ErrorIs(t, err, tc.want)
			if client != nil {
				require.NoError(t, client.Close())
			}
			require.Equal(t, 11, calls)
		})
	}
}
