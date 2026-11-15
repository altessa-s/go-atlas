// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"crypto/tls"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/types/redacted"
	"github.com/altessa-s/go-atlas/factory/store/mongo/kms"
	"github.com/altessa-s/go-atlas/store/mongo/kms"

	mongoconfig "github.com/altessa-s/go-atlas/config/mongo"
	kmsaws "github.com/altessa-s/go-atlas/store/mongo/kms/providers/aws"
	kmsazure "github.com/altessa-s/go-atlas/store/mongo/kms/providers/azure"
	kmsgcp "github.com/altessa-s/go-atlas/store/mongo/kms/providers/gcp"
	kmslocal "github.com/altessa-s/go-atlas/store/mongo/kms/providers/local"
)

var localMasterKey = strings.Repeat("k", kmslocal.RequiredMasterKeyLength)

func TestBuild_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  *mongoconfig.KMS
	}{
		{name: "nil config", cfg: nil},
		{name: "unsupported provider", cfg: &mongoconfig.KMS{Provider: "amazon"}},
		{name: "empty provider", cfg: &mongoconfig.KMS{}},
		{name: "local without section", cfg: &mongoconfig.KMS{Provider: mongoconfig.KMSProviderLocal}},
		{name: "aws without section", cfg: &mongoconfig.KMS{Provider: mongoconfig.KMSProviderAWS}},
		{name: "azure without section", cfg: &mongoconfig.KMS{Provider: mongoconfig.KMSProviderAzure}},
		{name: "gcp without section", cfg: &mongoconfig.KMS{Provider: mongoconfig.KMSProviderGCP}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p, err := factory.New(tc.cfg).Build()
			require.Error(t, err)
			require.Nil(t, p)
		})
	}
}

func TestBuild_LocalInvalidMasterKeyPropagatesProviderError(t *testing.T) {
	t.Parallel()

	cfg := &mongoconfig.KMS{
		Provider: mongoconfig.KMSProviderLocal,
		Local:    &mongoconfig.KMSLocal{MasterKey: "too-short"},
	}

	p, err := factory.New(cfg).Build()
	require.ErrorIs(t, err, kms.ErrConfigurationError)
	require.Nil(t, p)
}

func TestBuild_Local(t *testing.T) {
	t.Parallel()

	cfg := &mongoconfig.KMS{
		Provider: mongoconfig.KMSProviderLocal,
		Local:    &mongoconfig.KMSLocal{MasterKey: redacted.RedactedString(localMasterKey)},
	}

	p, err := factory.New(cfg).UseTlsConfig(&tls.Config{MinVersion: tls.VersionTLS13}).Build()
	require.NoError(t, err)
	t.Cleanup(p.Clear)

	require.Equal(t, kmslocal.ProviderName, p.Name())
	require.Equal(t, []byte(localMasterKey), p.Credentials()[kmslocal.ProviderName][kmslocal.MasterKey])
	require.Nil(t, p.TLSConfig(), "local provider ignores the builder TLS config")
}

func TestBuild_AWS(t *testing.T) {
	t.Parallel()

	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS13}
	cfg := &mongoconfig.KMS{
		Provider: mongoconfig.KMSProviderAWS,
		AWS: &mongoconfig.KMSAWS{
			AccessKeyId:     "id",
			SecretAccessKey: "secret",
			Key:             "arn:aws:kms:key",
			Region:          new("eu-west-1"),
			Endpoint:        new("kms.example.com"),
		},
	}

	p, err := factory.New(cfg).UseTlsConfig(tlsCfg).Build()
	require.NoError(t, err)
	t.Cleanup(p.Clear)

	require.Equal(t, kmsaws.ProviderName, p.Name())
	require.Same(t, tlsCfg, p.TLSConfig())
	require.Equal(t, kms.Key{
		kmsaws.KeyARN:   "arn:aws:kms:key",
		kmsaws.Region:   "eu-west-1",
		kmsaws.Endpoint: "kms.example.com",
	}, p.MasterKey())

	creds := p.Credentials()[kmsaws.ProviderName]
	require.Equal(t, "id", creds[kmsaws.AccessKeyID])
	require.Equal(t, "secret", creds[kmsaws.SecretAccessKey])
}

func TestBuild_AWSOptionalFieldsUnset(t *testing.T) {
	t.Parallel()

	cfg := &mongoconfig.KMS{
		Provider: mongoconfig.KMSProviderAWS,
		AWS:      &mongoconfig.KMSAWS{AccessKeyId: "id", SecretAccessKey: "secret", Key: "arn"},
	}

	p, err := factory.New(cfg).Build()
	require.NoError(t, err)
	t.Cleanup(p.Clear)

	require.Equal(t, kms.Key{kmsaws.KeyARN: "arn"}, p.MasterKey())
	require.Nil(t, p.TLSConfig())
}

func TestBuild_Azure(t *testing.T) {
	t.Parallel()

	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS13}
	cfg := &mongoconfig.KMS{
		Provider: mongoconfig.KMSProviderAzure,
		Azure: &mongoconfig.KMSAzure{
			ClientId:         "client",
			ClientSecret:     "secret",
			TenantId:         "tenant",
			KeyName:          "key",
			KeyVersion:       new("v2"),
			KeyVaultEndpoint: new("vault.example.net"),
		},
	}

	p, err := factory.New(cfg).UseTlsConfig(tlsCfg).Build()
	require.NoError(t, err)
	t.Cleanup(p.Clear)

	require.Equal(t, kmsazure.ProviderName, p.Name())
	require.Same(t, tlsCfg, p.TLSConfig())

	key := p.MasterKey()
	require.Equal(t, "key", key[kmsazure.AzureKeyName])
	require.Equal(t, "v2", key[kmsazure.AzureKeyVersion])

	creds := p.Credentials()[kmsazure.ProviderName]
	require.Equal(t, "client", creds[kmsazure.ClientID])
	require.Equal(t, "secret", creds[kmsazure.ClientSecret])
	require.Equal(t, "tenant", creds[kmsazure.TenantID])
}

func TestBuild_GCP(t *testing.T) {
	t.Parallel()

	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS13}
	cfg := &mongoconfig.KMS{
		Provider: mongoconfig.KMSProviderGCP,
		GCP: &mongoconfig.KMSGCP{
			ProjectId:              "project",
			Email:                  "sa@example.iam",
			PrivateKey:             "private",
			Location:               "global",
			KeyRing:                "ring",
			KeyName:                "key",
			KeyVersion:             new("3"),
			Endpoint:               new("kms.example.com"),
			AuthenticationEndpoint: new("auth.example.com"),
		},
	}

	p, err := factory.New(cfg).UseTlsConfig(tlsCfg).Build()
	require.NoError(t, err)
	t.Cleanup(p.Clear)

	require.Equal(t, kmsgcp.ProviderName, p.Name())
	require.Same(t, tlsCfg, p.TLSConfig())
	require.Equal(t, kms.Key{
		kmsgcp.ProjectID:   "project",
		kmsgcp.GCPLocation: "global",
		kmsgcp.KeyRing:     "ring",
		kmsgcp.GCPKeyName:  "key",
		kmsgcp.KeyVersion:  "3",
		kmsgcp.Endpoint:    "kms.example.com",
	}, p.MasterKey())

	creds := p.Credentials()[kmsgcp.ProviderName]
	require.Equal(t, "sa@example.iam", creds[kmsgcp.Email])
	require.Equal(t, "private", creds[kmsgcp.GCPPrivateKey])
}
