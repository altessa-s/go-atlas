// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/altessa-s/go-atlas/auth/oauth2client"
)

// followAll is a caller-supplied redirect policy that accepts every hop.
func followAll(*http.Request, []*http.Request) error { return nil }

// countingRedirectingIdP is redirectingIdP that also counts its own requests.
func countingRedirectingIdP(t *testing.T) (tokenURL string, hits, redirected *atomic.Int32) {
	t.Helper()
	hits, redirected = &atomic.Int32{}, &atomic.Int32{}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirected.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(tokenJSON))
	}))
	t.Cleanup(target.Close)
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(idp.Close)
	return idp.URL, hits, redirected
}

// An injected client must not carry the form, credentials included, to
// another origin when the endpoint answers with a 307.
func TestInjectedClientDoesNotFollowCrossOriginRedirects(t *testing.T) {
	t.Parallel()

	clients := map[string]struct {
		client      func() *http.Client
		wantRefused bool
	}{
		"no redirect policy":       {client: func() *http.Client { return &http.Client{} }},
		"permissive custom policy": {client: func() *http.Client { return &http.Client{CheckRedirect: followAll} }, wantRefused: true},
	}
	flows := map[string]func(t *testing.T, tokenURL string, opts ...oauth2client.Option) error{
		"x/oauth2 path": func(t *testing.T, tokenURL string, opts ...oauth2client.Option) error {
			opts = append(opts, oauth2client.WithAuthStyle(oauth2.AuthStyleInHeader))
			_, err := mustClientCredentials(t, tokenURL, "svc", "s3cr3t", opts...).Token()
			return err
		},
		"hand-rolled path": func(t *testing.T, tokenURL string, opts ...oauth2client.Option) error {
			_, err := mustExchanger(t, tokenURL, "svc", "s3cr3t", opts...).
				Exchange(t.Context(), oauth2client.ExchangeRequest{SubjectToken: "subject"})
			return err
		},
		"revocation": func(t *testing.T, tokenURL string, opts ...oauth2client.Option) error {
			return mustRevoker(t, tokenURL, "svc", "s3cr3t", opts...).Revoke(t.Context(), "refresh-token")
		},
	}

	for clientName, tc := range clients {
		for flowName, run := range flows {
			for _, insecure := range []bool{false, true} {
				name := clientName + "/" + flowName
				if insecure {
					name += "/insecure http allowed"
				}
				t.Run(name, func(t *testing.T) {
					t.Parallel()
					tokenURL, _, redirected := countingRedirectingIdP(t)
					injected := tc.client()
					opts := []oauth2client.Option{oauth2client.WithHTTPClient(injected)}
					if insecure {
						opts = append(opts, oauth2client.WithInsecureHTTP())
					}

					err := run(t, tokenURL, opts...)

					require.Error(t, err)
					require.Zero(t, redirected.Load(), "the form must not be re-posted to the redirect target")
					if tc.wantRefused {
						require.ErrorIs(t, err, oauth2client.ErrRedirectNotAllowed)
					}
				})
			}
		}
	}
}

// A caller-supplied redirect policy still decides about hops within the origin.
func TestInjectedClientPolicyDecidesSameOriginRedirects(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/v2/token", http.StatusTemporaryRedirect)
	})
	mux.HandleFunc("/v2/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(tokenJSON))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	var consulted atomic.Int32
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		consulted.Add(1)
		return nil
	}}

	tok, err := mustExchanger(t, srv.URL+"/token", "svc", "s3cr3t", oauth2client.WithHTTPClient(client)).
		Exchange(t.Context(), oauth2client.ExchangeRequest{SubjectToken: "subject"})

	require.NoError(t, err)
	require.Equal(t, "at", tok.AccessToken)
	require.Equal(t, int32(1), consulted.Load())
}

// A refused redirect fails the same way on every attempt, so the retry policy
// must not spend its attempts on it. (An insecure endpoint is rejected by the
// constructor and never reaches the retry policy.)
func TestPermanentRequestErrorsAreNotRetried(t *testing.T) {
	t.Parallel()
	retrying := []oauth2client.Option{
		oauth2client.WithRetryAttempts(3),
		oauth2client.WithRetryBaseDelay(time.Millisecond),
		oauth2client.WithRetryMaxDelay(time.Millisecond),
	}

	t.Run("token exchange, refused redirect", func(t *testing.T) {
		t.Parallel()
		tokenURL, hits, _ := countingRedirectingIdP(t)
		opts := append([]oauth2client.Option{oauth2client.WithHTTPClient(&http.Client{CheckRedirect: followAll})}, retrying...)

		_, err := mustExchanger(t, tokenURL, "svc", "s3cr3t", opts...).
			Exchange(t.Context(), oauth2client.ExchangeRequest{SubjectToken: "subject"})

		require.ErrorIs(t, err, oauth2client.ErrRedirectNotAllowed)
		require.ErrorIs(t, err, oauth2client.ErrTokenExchange)
		require.Equal(t, int32(1), hits.Load())
	})

	t.Run("jwt bearer, refused redirect", func(t *testing.T) {
		t.Parallel()
		tokenURL, hits, _ := countingRedirectingIdP(t)
		key, _ := rsaKey(t, "k")
		opts := append([]oauth2client.Option{oauth2client.WithHTTPClient(&http.Client{CheckRedirect: followAll})}, retrying...)
		b, err := oauth2client.NewJWTBearer(tokenURL, "svc", key, opts...)
		require.NoError(t, err)

		_, err = b.Token(t.Context())

		require.ErrorIs(t, err, oauth2client.ErrRedirectNotAllowed)
		require.ErrorIs(t, err, oauth2client.ErrTokenRequest)
		require.Equal(t, int32(1), hits.Load())
	})
}

// The constructors that can report an error reject an insecure endpoint up
// front; the authorization URL never passes through the guarded HTTP client.
func TestConstructorsValidateEndpointURLs(t *testing.T) {
	t.Parallel()
	const (
		secure   = "https://idp.example/x"
		insecure = "http://idp.example/x"
		loopback = "http://127.0.0.1:8080/x"
	)

	authCode := []struct {
		name     string
		endpoint oauth2.Endpoint
		opts     []oauth2client.Option
		wantErr  bool
	}{
		{name: "https", endpoint: oauth2.Endpoint{AuthURL: secure, TokenURL: secure}},
		{name: "loopback http", endpoint: oauth2.Endpoint{AuthURL: loopback, TokenURL: loopback}},
		{name: "insecure authorization URL", endpoint: oauth2.Endpoint{AuthURL: insecure, TokenURL: secure}, wantErr: true},
		{name: "insecure token URL", endpoint: oauth2.Endpoint{AuthURL: secure, TokenURL: insecure}, wantErr: true},
		{
			name:     "insecure allowed explicitly",
			endpoint: oauth2.Endpoint{AuthURL: insecure, TokenURL: insecure},
			opts:     []oauth2client.Option{oauth2client.WithInsecureHTTP()},
		},
	}
	for _, tc := range authCode {
		t.Run("auth code/"+tc.name, func(t *testing.T) {
			t.Parallel()
			ac, err := oauth2client.NewAuthCode(tc.endpoint, "svc", "s3cr3t", "https://app.example/cb", tc.opts...)
			if tc.wantErr {
				require.ErrorIs(t, err, oauth2client.ErrInsecureEndpoint)
				require.Nil(t, ac)
				return
			}
			require.NoError(t, err)
			require.NotEmpty(t, ac.AuthCodeURL("state"))
		})
	}

	t.Run("jwt bearer", func(t *testing.T) {
		t.Parallel()
		key, _ := rsaKey(t, "k")

		_, err := oauth2client.NewJWTBearer(insecure, "svc", key)
		require.ErrorIs(t, err, oauth2client.ErrInsecureEndpoint)
		require.ErrorIs(t, err, oauth2client.ErrJWTBearerConfig)

		_, err = oauth2client.NewJWTBearer(insecure, "svc", key, oauth2client.WithInsecureHTTP())
		require.NoError(t, err)
		_, err = oauth2client.NewJWTBearer(loopback, "svc", key)
		require.NoError(t, err)
	})
}
