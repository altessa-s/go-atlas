// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factoryconv_test

import (
	"crypto/tls"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-tlsutils/providers/file"

	"github.com/altessa-s/go-atlas/factory/transport/internal/factoryconv"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	tlsconfig "github.com/altessa-s/go-atlas/config/tls"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	tlsproviders "github.com/altessa-s/go-tlsutils/providers"
)

func TestServerTLS(t *testing.T) {
	t.Parallel()
	base := corefactory.NewBase(nil)
	cfg := &tlsconfig.Server{Provider: tlsconfig.ProviderTypeFile, MinTLSVersion: "1.3"}
	_, err := factoryconv.ServerTLS(&base, nil, cfg)
	require.ErrorIs(t, err, corefactory.ErrDependencyRequired)

	cert, key, _ := testhelpers.SelfSignedCert(t)
	certFile, keyFile := testhelpers.WriteTempCertFiles(t, cert, key)
	provider, err := tlsfile.NewWithCertAndKey(certFile, keyFile, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, provider.Close(t.Context())) })
	var providers tlsproviders.Providers
	providers.Register(provider)
	got, err := factoryconv.ServerTLS(&base, &providers, cfg)
	require.NoError(t, err)
	require.Equal(t, uint16(tls.VersionTLS13), got.MinVersion)
}
