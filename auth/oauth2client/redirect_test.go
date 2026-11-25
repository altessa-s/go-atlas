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

	"github.com/altessa-s/go-atlas/auth/oauth2client"

	"golang.org/x/oauth2"
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
	flows := map[string]func(t *testing.T, tokenURL string, client *http.Client, allowHTTP bool) error{
		"x/oauth2 path": func(t *testing.T, tokenURL string, client *http.Client, allowHTTP bool) error {
			_, err := mustClientCredentials(t, tokenURL, "svc", "s3cr3t", allowInsecureHTTP(allowHTTP,
				oauth2client.WithClientCredentialsInsecureHTTP,
				oauth2client.WithClientCredentialsHTTPClient(client),
				oauth2client.WithClientCredentialsAuthStyle(oauth2.AuthStyleInHeader))...).Token()
			return err
		},
		"hand-rolled path": func(t *testing.T, tokenURL string, client *http.Client, allowHTTP bool) error {
			_, err := mustExchanger(t, tokenURL, "svc", "s3cr3t", allowInsecureHTTP(allowHTTP,
				oauth2client.WithExchangerInsecureHTTP,
				oauth2client.WithExchangerHTTPClient(client))...).
				Exchange(t.Context(), oauth2client.ExchangeRequest{SubjectToken: "subject"})
			return err
		},
		"revocation": func(t *testing.T, tokenURL string, client *http.Client, allowHTTP bool) error {
			return mustRevoker(t, tokenURL, "svc", "s3cr3t", allowInsecureHTTP(allowHTTP,
				oauth2client.WithRevokerInsecureHTTP,
				oauth2client.WithRevokerHTTPClient(client))...).Revoke(t.Context(), "refresh-token")
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
					err := run(t, tokenURL, tc.client(), insecure)

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

	tok, err := mustExchanger(t, srv.URL+"/token", "svc", "s3cr3t", oauth2client.WithExchangerHTTPClient(client)).
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

	t.Run("token exchange, refused redirect", func(t *testing.T) {
		t.Parallel()
		tokenURL, hits, _ := countingRedirectingIdP(t)
		opts := []oauth2client.ExchangerOption{
			oauth2client.WithExchangerHTTPClient(&http.Client{CheckRedirect: followAll}),
			oauth2client.WithExchangerRetryAttempts(3),
			oauth2client.WithExchangerRetryBaseDelay(time.Millisecond),
			oauth2client.WithExchangerRetryMaxDelay(time.Millisecond),
		}

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
		opts := []oauth2client.JWTBearerOption{
			oauth2client.WithJWTBearerHTTPClient(&http.Client{CheckRedirect: followAll}),
			oauth2client.WithJWTBearerRetryAttempts(3),
			oauth2client.WithJWTBearerRetryBaseDelay(time.Millisecond),
			oauth2client.WithJWTBearerRetryMaxDelay(time.Millisecond),
		}
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
		opts     []oauth2client.AuthCodeOption
		wantErr  bool
	}{
		{name: "https", endpoint: oauth2.Endpoint{AuthURL: secure, TokenURL: secure}},
		{name: "loopback http", endpoint: oauth2.Endpoint{AuthURL: loopback, TokenURL: loopback}},
		{name: "insecure authorization URL", endpoint: oauth2.Endpoint{AuthURL: insecure, TokenURL: secure}, wantErr: true},
		{name: "insecure token URL", endpoint: oauth2.Endpoint{AuthURL: secure, TokenURL: insecure}, wantErr: true},
		{
			name:     "insecure allowed explicitly",
			endpoint: oauth2.Endpoint{AuthURL: insecure, TokenURL: insecure},
			opts:     []oauth2client.AuthCodeOption{oauth2client.WithAuthCodeInsecureHTTP()},
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

		_, err = oauth2client.NewJWTBearer(insecure, "svc", key, oauth2client.WithJWTBearerInsecureHTTP())
		require.NoError(t, err)
		_, err = oauth2client.NewJWTBearer(loopback, "svc", key)
		require.NoError(t, err)
	})
}
