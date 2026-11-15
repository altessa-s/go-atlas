// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	coremtls "github.com/altessa-s/go-atlas/auth/mtls"
	authconfig "github.com/altessa-s/go-atlas/config/auth"
	factory "github.com/altessa-s/go-atlas/factory/auth/mtls"
	grpcmtls "github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth/mtls"
	httpmtls "github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth/mtls"
)

// A configuration without a trust decision yields no authenticator.
func TestAuthenticatorFailsWithoutTrustDecision(t *testing.T) {
	t.Parallel()
	a, err := factory.New(&authconfig.MTLS{CheckExpiry: true, AllowedSubjectCNs: []string{"billing"}}).Authenticator()
	require.ErrorIs(t, err, coremtls.ErrNoTrustDomainPolicy)
	require.Nil(t, a)
}

// The options of a valid configuration construct both transport adapters.
func TestOptionsConstructTransportAdapters(t *testing.T) {
	t.Parallel()

	for _, cfg := range []authconfig.MTLS{
		{TrustDomains: []string{"example.org"}},
		{AllowAnyTrustDomain: true},
	} {
		opts, err := factory.New(&cfg).Options()
		require.NoError(t, err)

		authFn, err := grpcmtls.AuthFunc(opts...)
		require.NoError(t, err)
		require.NotNil(t, authFn)

		mw, err := httpmtls.Middleware(opts...)
		require.NoError(t, err)
		require.NotNil(t, mw)
	}
}
