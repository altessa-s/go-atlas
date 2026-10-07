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

func TestNewProvider_UsesInjectedClient(t *testing.T) {
	t.Parallel()

	srv := stubOIDCServer(t)
	client := httpclient.New(httpclient.WithRetryMax(0), httpclient.WithoutProxy())
	p, err := NewProvider(t.Context(), srv.URL,
		WithHTTPClient(client),
		// httptest serves plain-HTTP on loopback; disable discovery endpoint
		// pinning for the stub IdP.
		WithDiscoveryValidationMode(DiscoveryValidationModeDisabled),
	)
	require.NoError(t, err)
	t.Cleanup(p.Close)

	require.Same(t, client, p.client)
}

func TestNewProvider_RequiresClient(t *testing.T) {
	t.Parallel()

	srv := stubOIDCServer(t)
	_, err := NewProvider(t.Context(), srv.URL, WithDiscoveryValidationMode(DiscoveryValidationModeDisabled))
	require.ErrorIs(t, err, ErrHTTPClientRequired)
}

func TestNewProvider_InjectsClientIntoURLRevocationLoader(t *testing.T) {
	t.Parallel()

	srv := stubOIDCServer(t)
	loader := &URLRevocationLoader{URL: "http://127.0.0.1:1/revocations"}
	require.Nil(t, loader.Client, "precondition: loader has no client")

	p, err := NewProvider(t.Context(), srv.URL,
		WithHTTPClient(httpclient.New(httpclient.WithRetryMax(0))),
		WithRevocationLoader(loader),
		WithRevocationFilter(noopFilter{}),
		// The stub filter cannot rebuild and the loader URL is unreachable,
		// so the initial revocation sync fails; tolerate it to reach the
		// injection assertion.
		WithRevocationFailOpen(),
		// httptest serves plain-HTTP on loopback; disable discovery endpoint
		// pinning for the stub IdP.
		WithDiscoveryValidationMode(DiscoveryValidationModeDisabled),
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
