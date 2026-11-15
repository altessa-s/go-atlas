// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package token_test

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/security/vault/auth"
	"github.com/altessa-s/go-atlas/security/vault/auth/token"

	vaultApi "github.com/hashicorp/vault/api"
)

// Shutdown may run while logins are in flight: every login must either use
// the token or report the shutdown, without a data race or a nil dereference.
// Run with -race.
func TestAuthenticate_ConcurrentWithShutdown(t *testing.T) {
	t.Parallel()
	transport := testhelpers.RoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"data":{"renewable":false,"ttl":0}}`)),
			Request:    req,
		}, nil
	})
	m := token.New("hvs.token")

	const logins = 32
	errs := make([]error, logins)
	var wg sync.WaitGroup
	for i := range logins {
		wg.Go(func() {
			client, err := vaultApi.NewClient(&vaultApi.Config{
				Address: "http://vault.test", MaxRetries: 0, HttpClient: &http.Client{Transport: transport},
			})
			if err != nil {
				errs[i] = err
				return
			}
			_, errs[i] = m.Authenticate(t.Context(), client)
		})
	}
	wg.Go(func() { _ = m.Shutdown() })
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			require.ErrorIs(t, err, auth.ErrMethodShutDown)
		}
	}
	_, err := m.Authenticate(t.Context(), nil)
	require.True(t, errors.Is(err, auth.ErrMethodShutDown))
}
