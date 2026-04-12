// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package kmsazure_test

import (
	"crypto/tls"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/mongo/kms"

	kmsazure "github.com/altessa-s/go-atlas/data/mongo/kms/azure"
)

func TestNew(t *testing.T) {
	p := kmsazure.New("client-id", "client-secret", "tenant-id", "my-key")
	require.NotNil(t, p)
}

func TestAzure_Name(t *testing.T) {
	p := kmsazure.New("c", "s", "t", "k")
	require.Equal(t, "azure", p.Name())
}

func TestAzure_Credentials(t *testing.T) {
	p := kmsazure.New("client-id", "client-secret", "tenant-id", "key")
	creds := p.Credentials()

	azCreds, ok := creds["azure"]
	require.True(t, ok, "Credentials() missing 'azure' key")
	require.Equal(t, "client-id", azCreds[kmsazure.ClientID])
	require.Equal(t, "client-secret", azCreds[kmsazure.ClientSecret])
	require.Equal(t, "tenant-id", azCreds[kmsazure.TenantID])
}

func TestAzure_MasterKey(t *testing.T) {
	version := "1"
	endpoint := "https://vault.vault.azure.net"
	p := kmsazure.New("c", "s", "t", "my-key",
		kmsazure.WithKeyVersion(&version),
		kmsazure.WithKeyVaultEndpoint(&endpoint),
	)

	key := p.MasterKey()
	require.Equal(t, "my-key", key[kmsazure.AzureKeyName])
	require.Equal(t, "1", key[kmsazure.AzureKeyVersion])
}

func TestAzure_MasterKey_NoOptionals(t *testing.T) {
	p := kmsazure.New("c", "s", "t", "key")
	key := p.MasterKey()
	_, hasVersion := key[kmsazure.AzureKeyVersion]
	require.False(t, hasVersion)
}

func TestAzure_TLSConfig_Nil(t *testing.T) {
	p := kmsazure.New("c", "s", "t", "k")
	require.Nil(t, p.TLSConfig())
}

func TestAzure_TLSConfig_Custom(t *testing.T) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS13} //nolint:gosec
	p := kmsazure.New("c", "s", "t", "k", kmsazure.WithTLS(tlsCfg))
	require.Equal(t, tlsCfg, p.TLSConfig())
}

func TestAzure_Clear(t *testing.T) {
	p := kmsazure.New("c", "s", "t", "k")
	p.Clear()
	creds := p.Credentials()
	require.Len(t, creds["azure"], 0)
}

func TestAzure_ImplementsProvider(t *testing.T) {
	var _ kms.Provider = kmsazure.New("c", "s", "t", "k")
}
