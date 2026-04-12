// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package kmsgcp_test

import (
	"crypto/tls"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/mongo/kms"

	kmsgcp "github.com/altessa-s/go-atlas/data/mongo/kms/gcp"
)

func TestNew(t *testing.T) {
	p := kmsgcp.New("project", "email@sa.com", "privkey", "global", "ring", "key")
	require.NotNil(t, p)
}

func TestGoogle_Name(t *testing.T) {
	p := kmsgcp.New("p", "e", "k", "l", "r", "n")
	require.Equal(t, "gcp", p.Name())
}

func TestGoogle_Credentials(t *testing.T) {
	p := kmsgcp.New("project", "email@test.com", "private-key", "global", "ring", "key")
	creds := p.Credentials()

	gcpCreds, ok := creds["gcp"]
	require.True(t, ok, "Credentials() missing 'gcp' key")
	require.Equal(t, "email@test.com", gcpCreds[kmsgcp.Email])
	require.Equal(t, "private-key", gcpCreds[kmsgcp.GCPPrivateKey])
}

func TestGoogle_Credentials_WithAuthEndpoint(t *testing.T) {
	authEp := "https://custom-auth.com"
	p := kmsgcp.New("p", "e", "k", "l", "r", "n",
		kmsgcp.WithAuthenticationEndpoint(&authEp),
	)
	creds := p.Credentials()
	require.Equal(t, "https://custom-auth.com", creds["gcp"][kmsgcp.Endpoint])
}

func TestGoogle_MasterKey(t *testing.T) {
	version := "1"
	endpoint := "https://custom.kms.com"
	p := kmsgcp.New("project", "e", "k", "global", "ring", "key",
		kmsgcp.WithKeyVersion(&version),
		kmsgcp.WithEndpoint(&endpoint),
	)

	key := p.MasterKey()
	require.Equal(t, "project", key[kmsgcp.ProjectID])
	require.Equal(t, "global", key[kmsgcp.GCPLocation])
	require.Equal(t, "ring", key[kmsgcp.KeyRing])
	require.Equal(t, "key", key[kmsgcp.GCPKeyName])
	require.Equal(t, "1", key[kmsgcp.KeyVersion])
	require.Equal(t, "https://custom.kms.com", key[kmsgcp.Endpoint])
}

func TestGoogle_MasterKey_NoOptionals(t *testing.T) {
	p := kmsgcp.New("p", "e", "k", "l", "r", "n")
	key := p.MasterKey()
	_, hasVersion := key[kmsgcp.KeyVersion]
	require.False(t, hasVersion)
	_, hasEndpoint := key[kmsgcp.Endpoint]
	require.False(t, hasEndpoint)
}

func TestGoogle_TLSConfig_Nil(t *testing.T) {
	p := kmsgcp.New("p", "e", "k", "l", "r", "n")
	require.Nil(t, p.TLSConfig())
}

func TestGoogle_TLSConfig_Custom(t *testing.T) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS13} //nolint:gosec
	p := kmsgcp.New("p", "e", "k", "l", "r", "n", kmsgcp.WithTLS(tlsCfg))
	require.Equal(t, tlsCfg, p.TLSConfig())
}

func TestGoogle_Clear(t *testing.T) {
	p := kmsgcp.New("p", "e", "k", "l", "r", "n")
	p.Clear()
	creds := p.Credentials()
	require.Len(t, creds["gcp"], 0)
}

func TestGoogle_ImplementsProvider(t *testing.T) {
	var _ kms.Provider = kmsgcp.New("p", "e", "k", "l", "r", "n")
}
