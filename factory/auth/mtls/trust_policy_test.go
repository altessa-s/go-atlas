// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	coremtls "github.com/altessa-s/go-atlas/auth/mtls"
	authconfig "github.com/altessa-s/go-atlas/config/auth"
	factory "github.com/altessa-s/go-atlas/factory/auth/mtls"
)

// The configured trust domains are the authenticator's trust decision, not an
// ancillary validator: they hold on their own, with every other check off, and
// an extra opt-in passed by the caller does not widen them.
func TestTrustDomainsAreTheTrustDecision(t *testing.T) {
	t.Parallel()
	farFuture := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	cfg := &authconfig.MTLS{TrustDomains: []string{"example.org"}, CheckExpiry: false}

	opts, err := factory.New(cfg).Options()
	require.NoError(t, err)
	require.Len(t, opts, 1)

	a, err := factory.New(cfg).Authenticator(coremtls.WithInsecureAnyTrustDomain())
	require.NoError(t, err)

	_, err = a.Authenticate(t.Context(), cert(t, "spiffe://example.org/x", farFuture))
	require.NoError(t, err)
	_, err = a.Authenticate(t.Context(), cert(t, "spiffe://foreign.example/x", farFuture))
	require.ErrorIs(t, err, coremtls.ErrUntrustedDomain)
}
