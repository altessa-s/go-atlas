// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"fmt"
	"iter"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	httpclient "github.com/altessa-s/go-atlas/transport/http/client"
)

// stubOIDCServer answers the OIDC discovery probe AND the JWKS endpoint
// it advertises. Both responses are minimal but valid — enough for
// NewProvider to complete its happy path without standing up a full IdP.
func stubOIDCServer(t *testing.T) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/jwks":
			_, _ = w.Write([]byte(`{"keys":[]}`))
		default:
			_, _ = fmt.Fprintf(w, `{
				"issuer": %q,
				"authorization_endpoint": %q,
				"token_endpoint": %q,
				"jwks_uri": %q,
				"userinfo_endpoint": %q,
				"id_token_signing_alg_values_supported": ["RS256"]
			}`, srv.URL, srv.URL+"/authorize", srv.URL+"/token", srv.URL+"/jwks", srv.URL+"/userinfo")
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestNewOptions_StoresHTTPClientOptions(t *testing.T) {
	t.Parallel()

	o := newOptions(WithHTTPClientOptions())
	require.Empty(t, o.httpClientOptions)

	o = newOptions(WithHTTPClientOptions(httpclient.WithoutProxy(), httpclient.WithoutProxy()))
	require.Len(t, o.httpClientOptions, 2)
}

func TestNewProvider_BuildsResilientClient(t *testing.T) {
	t.Parallel()

	srv := stubOIDCServer(t)
	p, err := NewProvider(t.Context(), srv.URL,
		WithHTTPClientOptions(httpclient.WithRetryMax(0), httpclient.WithoutProxy()),
	)
	require.NoError(t, err)
	t.Cleanup(p.Close)

	require.NotNil(t, p.client)
	require.NotSame(t, http.DefaultClient, p.client,
		"Provider must build a resilient client from httpClientOptions, not reuse http.DefaultClient")
}

func TestNewProvider_InjectsClientIntoURLRevocationLoader(t *testing.T) {
	t.Parallel()

	srv := stubOIDCServer(t)
	loader := &URLRevocationLoader{URL: "http://127.0.0.1:1/revocations"}
	require.Nil(t, loader.Client, "precondition: loader has no client")

	p, err := NewProvider(t.Context(), srv.URL,
		WithHTTPClientOptions(httpclient.WithRetryMax(0)),
		WithRevocationLoader(loader),
		WithRevocationFilter(noopFilter{}),
	)
	require.NoError(t, err)
	t.Cleanup(p.Close)

	require.Same(t, p.client, loader.Client,
		"Provider must inject its shared HTTP client into the URLRevocationLoader")
}

// noopFilter is a tiny stub Filter so NewProvider can wire revocation
// without requiring a real Redis/probabilistic-filter dependency.
type noopFilter struct{}

func (noopFilter) MightExist(context.Context, string) (bool, error) { return false, nil }
func (noopFilter) Add(context.Context, string) error                { return nil }
func (noopFilter) AddBatch(context.Context, iter.Seq[string]) error { return nil }
