// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mtls_test

import (
	"crypto/x509"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/principal"

	coremtls "github.com/altessa-s/go-atlas/auth/mtls"
)

func TestPrincipalIdentity(t *testing.T) {
	t.Parallel()
	u, err := url.Parse("spiffe://example.org/sa/billing")
	require.NoError(t, err)
	cert := &x509.Certificate{URIs: []*url.URL{u}}

	got, err := coremtls.PrincipalIdentity(cert)
	require.NoError(t, err)
	p, ok := got.(principal.Principal)
	require.True(t, ok)
	require.Equal(t, "spiffe://example.org/sa/billing", p.Subject)
	require.Empty(t, p.Scopes)
}

func TestPrincipalIdentityNoSPIFFE(t *testing.T) {
	t.Parallel()
	_, err := coremtls.PrincipalIdentity(&x509.Certificate{})
	require.Error(t, err) // no SPIFFE URI SAN
}
