// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongoconfig_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/types/redacted"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	mongoconfig "github.com/altessa-s/go-atlas/config/mongo"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

func fieldErrors(t *testing.T, err error) validation.Errors {
	t.Helper()
	verrs, ok := errors.AsType[validation.Errors](err)
	require.True(t, ok, "want validation.Errors, got %v", err)
	return verrs
}

func validConfig() mongoconfig.Config {
	c := mongoconfig.Default()
	c.Database = "app"
	return c
}

func TestDefault(t *testing.T) {
	t.Parallel()

	c := mongoconfig.Default()
	require.Equal(t, []string{"localhost:27017"}, c.Hosts)
	require.True(t, c.RetryReads)
	require.True(t, c.RetryWrites)
	require.False(t, c.UseConnectionURI())
	require.Contains(t, fieldErrors(t, c.Validate()), "Database", "database has no default")
	cfg := validConfig()
	require.NoError(t, cfg.Validate())
}

func TestCompressionTypes_StringsSlice(t *testing.T) {
	t.Parallel()

	cts := mongoconfig.CompressionTypes{mongoconfig.CompressionTypeSnappy, mongoconfig.CompressionTypeZstd}
	require.Equal(t, []string{"snappy", "zstd"}, cts.StringsSlice())
	require.Empty(t, mongoconfig.CompressionTypes(nil).StringsSlice())
	require.Equal(t, "MONGODB-X509", mongoconfig.AuthMechanismTypeX509.String())
}

func TestConfig_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		mut   func(*mongoconfig.Config)
		field string
	}{
		{"no hosts", func(c *mongoconfig.Config) { c.Hosts = nil }, "Hosts"},
		{"direct connection with many hosts", func(c *mongoconfig.Config) {
			c.DirectConnection = true
			c.Hosts = []string{"a:27017", "b:27017"}
		}, "DirectConnection"},
		{"unknown compressor", func(c *mongoconfig.Config) {
			c.Compressors = mongoconfig.CompressionTypes{"lz4"}
		}, "Compressors"},
		{"x509 without tls", func(c *mongoconfig.Config) {
			c.Credentials = &mongoconfig.Credentials{AuthMechanism: mongoconfig.AuthMechanismTypeX509}
		}, "TLS"},
		{"scram without its block", func(c *mongoconfig.Config) {
			c.Credentials = &mongoconfig.Credentials{AuthMechanism: mongoconfig.AuthMechanismTypeSCRAMSHA256}
		}, "Credentials"},
		{"scram without password", func(c *mongoconfig.Config) {
			c.Credentials = &mongoconfig.Credentials{
				AuthMechanism: mongoconfig.AuthMechanismTypeSCRAMSHA256,
				Scram:         &mongoconfig.SCRAMCredentials{Username: "u", AuthSource: "admin"},
			}
		}, "Credentials"},
		{"unknown auth mechanism", func(c *mongoconfig.Config) {
			c.Credentials = &mongoconfig.Credentials{AuthMechanism: "KERBEROS"}
		}, "Credentials"},
		{"invalid encryption", func(c *mongoconfig.Config) {
			c.Encryption = &mongoconfig.Encryption{Type: mongoconfig.EncryptionTypeAuto}
		}, "Encryption"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := validConfig()
			tc.mut(&c)
			require.Contains(t, fieldErrors(t, c.Validate()), tc.field)
		})
	}

	t.Run("direct connection with a single host", func(t *testing.T) {
		t.Parallel()
		c := validConfig()
		c.DirectConnection = true
		c.Compressors = mongoconfig.CompressionTypes{mongoconfig.CompressionTypeZlib}
		require.NoError(t, c.Validate())
	})
}

func TestConfig_ConnectionURI(t *testing.T) {
	t.Parallel()

	c := validConfig()
	c.Hosts = nil
	c.ConnectionURI = "mongodb://localhost:27017"
	require.True(t, c.UseConnectionURI())
	require.NoError(t, c.Validate())

	c.Credentials = &mongoconfig.Credentials{}
	require.ErrorIs(t, c.Validate(), coreerrs.ErrInvalidArgument)
}

func TestCredentials_Validate(t *testing.T) {
	t.Parallel()

	scram := &mongoconfig.SCRAMCredentials{Username: "u", Password: "p", AuthSource: "admin"}
	require.NoError(t, (&mongoconfig.Credentials{
		AuthMechanism: mongoconfig.AuthMechanismTypeSCRAMSHA256, Scram: scram,
	}).Validate())
	require.NoError(t, (&mongoconfig.Credentials{
		AuthMechanism: mongoconfig.AuthMechanismTypePLAIN,
		Plain:         &mongoconfig.PLAINCredentials{Username: "u", Password: "p"},
	}).Validate())
	require.NoError(t, (&mongoconfig.Credentials{AuthMechanism: mongoconfig.AuthMechanismTypeX509}).Validate())

	require.Contains(t, fieldErrors(t, (&mongoconfig.Credentials{AuthMechanism: "KERBEROS"}).Validate()), "AuthMechanism")
	require.Contains(t, fieldErrors(t, (&mongoconfig.Credentials{
		AuthMechanism: mongoconfig.AuthMechanismTypePLAIN,
	}).Validate()), "Plain")
	require.Contains(t, fieldErrors(t, (&mongoconfig.Credentials{
		AuthMechanism: mongoconfig.AuthMechanismTypeSCRAMSHA1,
	}).Validate()), "Scram")

	verrs := fieldErrors(t, (&mongoconfig.SCRAMCredentials{}).Validate())
	require.Contains(t, verrs, "Username")
	require.Contains(t, verrs, "Password")
	require.Contains(t, verrs, "AuthSource")

	verrs = fieldErrors(t, (&mongoconfig.PLAINCredentials{}).Validate())
	require.Contains(t, verrs, "Username")
	require.Contains(t, verrs, "Password")
}

func TestKMS_Validate(t *testing.T) {
	t.Parallel()

	require.Contains(t, fieldErrors(t, (&mongoconfig.KMS{}).Validate()), "Provider")
	require.Contains(t, fieldErrors(t, (&mongoconfig.KMS{Provider: "vault"}).Validate()), "Provider")

	for provider, field := range map[mongoconfig.KMSProvider]string{
		mongoconfig.KMSProviderLocal: "Local",
		mongoconfig.KMSProviderAWS:   "AWS",
		mongoconfig.KMSProviderAzure: "Azure",
		mongoconfig.KMSProviderGCP:   "GCP",
	} {
		require.Contains(t, fieldErrors(t, (&mongoconfig.KMS{Provider: provider}).Validate()), field, provider)
	}

	key := redacted.RedactedString(strings.Repeat("k", mongoconfig.MasterKeyLength))
	require.NoError(t, (&mongoconfig.KMS{
		Provider: mongoconfig.KMSProviderLocal, Local: &mongoconfig.KMSLocal{MasterKey: key},
	}).Validate())
}

func TestKMSLocal_Validate(t *testing.T) {
	t.Parallel()

	keyFile := filepath.Join(t.TempDir(), "master.key")
	require.NoError(t, os.WriteFile(keyFile, []byte("x"), 0o600))

	require.NoError(t, (&mongoconfig.KMSLocal{MasterKeyFile: keyFile}).Validate())

	verrs := fieldErrors(t, (&mongoconfig.KMSLocal{}).Validate())
	require.Contains(t, verrs, "MasterKey")
	require.Contains(t, verrs, "MasterKeyFile")

	require.Contains(t, fieldErrors(t, (&mongoconfig.KMSLocal{MasterKey: "short"}).Validate()), "MasterKey")
	require.Contains(t, fieldErrors(t, (&mongoconfig.KMSLocal{
		MasterKeyFile: filepath.Join(t.TempDir(), "missing.key"),
	}).Validate()), "MasterKeyFile")
}

func TestKMSAWS_Validate(t *testing.T) {
	t.Parallel()

	require.NoError(t, (&mongoconfig.KMSAWS{AccessKeyId: "a", SecretAccessKey: "s", Key: "k", Region: new("us-east-1")}).Validate())

	verrs := fieldErrors(t, (&mongoconfig.KMSAWS{Region: new("")}).Validate())
	for _, f := range []string{"AccessKeyId", "SecretAccessKey", "Key", "Region"} {
		require.Contains(t, verrs, f)
	}
}

func TestKMSAzure_Validate(t *testing.T) {
	t.Parallel()

	require.NoError(t, (&mongoconfig.KMSAzure{
		ClientId: "c", ClientSecret: "s", TenantId: "t", KeyName: "k", KeyVaultEndpoint: new("https://kv"),
	}).Validate())

	verrs := fieldErrors(t, (&mongoconfig.KMSAzure{}).Validate())
	for _, f := range []string{"ClientId", "ClientSecret", "TenantId", "KeyName", "KeyVaultEndpoint"} {
		require.Contains(t, verrs, f)
	}
}

func TestKMSGCP_Validate(t *testing.T) {
	t.Parallel()

	require.NoError(t, (&mongoconfig.KMSGCP{
		ProjectId: "p", Email: "e", PrivateKey: "k", Location: "l", KeyRing: "r", KeyName: "n",
	}).Validate())

	verrs := fieldErrors(t, (&mongoconfig.KMSGCP{Endpoint: new("")}).Validate())
	for _, f := range []string{"Endpoint", "ProjectId", "Email", "PrivateKey", "Location", "KeyRing", "KeyName"} {
		require.Contains(t, verrs, f)
	}
}

func TestEncryption_Validate(t *testing.T) {
	t.Parallel()

	key := redacted.RedactedString(strings.Repeat("k", mongoconfig.MasterKeyLength))
	valid := mongoconfig.Encryption{
		VaultCollection: "__keyVault",
		Type:            mongoconfig.EncryptionTypeManual,
		KMS:             &mongoconfig.KMS{Provider: mongoconfig.KMSProviderLocal, Local: &mongoconfig.KMSLocal{MasterKey: key}},
	}
	require.NoError(t, valid.Validate())

	verrs := fieldErrors(t, (&mongoconfig.Encryption{Type: "both", VaultDatabase: new("")}).Validate())
	for _, f := range []string{"VaultDatabase", "VaultCollection", "KMS", "Type"} {
		require.Contains(t, verrs, f)
	}
}
